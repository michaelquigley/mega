// Package events is the per-conversation append-only event history.
package events

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/michaelquigley/df/dd"
	"github.com/michaelquigley/df/dl"

	"git.hq.quigley.com/research/roundhouse/internal/durable"
	"git.hq.quigley.com/research/roundhouse/internal/model"
)

// ErrUncertain marks a log that no longer accepts appends because a failed
// append could not be rolled back, leaving unconfirmed bytes on disk.
var ErrUncertain = errors.New("event log durability uncertain after a failed rollback, restart required")

// Log is one conversation's JSONL event history. it has a single writer, the
// service, and assigns each appended event the next seq. size is the confirmed
// length; reads never go past it.
type Log struct {
	mu        sync.Mutex
	path      string
	file      *os.File
	size      int64
	last      int64
	uncertain error
}

// Open opens or creates the log at path. a final line with no newline can only
// be a write torn by a crash, since each append writes its newline last, so it
// is dropped; any other unreadable line is an error and is left in place.
func Open(path string) (*Log, error) {
	_, statErr := os.Stat(path)
	if statErr != nil && !errors.Is(statErr, os.ErrNotExist) {
		return nil, statErr
	}
	dir := filepath.Dir(path)
	if err := durable.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	file, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE|os.O_APPEND, 0o600)
	if err != nil {
		return nil, err
	}
	// appends sync the file's bytes, not the entry naming it; without this a new
	// conversation's synced events could survive a crash while its file did not.
	if statErr != nil {
		if err := durable.SyncDir(dir); err != nil {
			file.Close()
			return nil, fmt.Errorf("syncing directory of '%s': %w", path, err)
		}
	}
	info, err := file.Stat()
	if err != nil {
		file.Close()
		return nil, err
	}
	var last, size int64
	err = scan(file, info.Size(), func(ev *model.Event, end int64) bool {
		last, size = ev.Seq, end
		return true
	})
	if err != nil {
		file.Close()
		return nil, fmt.Errorf("reading event log '%s': %w", path, err)
	}
	if info.Size() != size {
		dl.Warnf("dropping %d torn bytes from event log '%s'", info.Size()-size, path)
		if err := file.Truncate(size); err != nil {
			file.Close()
			return nil, err
		}
	}
	return &Log{path: path, file: file, size: size, last: last}, nil
}

// scan reads complete lines from the first limit bytes of file, calling fn with
// each event and the offset just past its newline, until fn returns false or the
// data ends. it reads by offset, so it never moves the file's position.
func scan(file *os.File, limit int64, fn func(ev *model.Event, end int64) bool) error {
	reader := bufio.NewReader(io.NewSectionReader(file, 0, limit))
	var offset int64
	for number := 1; ; number++ {
		line, err := reader.ReadBytes('\n')
		if errors.Is(err, io.EOF) {
			return nil // anything in line here is an unterminated tail
		}
		if err != nil {
			return err
		}
		offset += int64(len(line))
		ev := &model.Event{}
		if err := dd.BindJSON(ev, bytes.TrimSpace(line)); err != nil {
			return fmt.Errorf("line %d: %w", number, err)
		}
		if !fn(ev, offset) {
			return nil
		}
	}
}

// Append assigns the next seq, stamps At when unset, and writes the event. it
// returns only once the event is on disk, and on failure leaves the log, its
// seq, and ev as they were, so nothing a caller confirms can be lost by a crash.
//
// a failed append is truncated back and can be retried. if the truncate fails
// too, unconfirmed bytes may sit past the confirmed length: reads keep serving
// what was confirmed, and every later append is refused with ErrUncertain until
// a restart reopens the log, where the torn-tail rule handles the remainder.
func (l *Log) Append(ev *model.Event) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.uncertain != nil {
		return l.uncertain
	}

	stamped := *ev
	stamped.Seq = l.last + 1
	if stamped.At.IsZero() {
		stamped.At = time.Now().UTC()
	}
	line, err := dd.UnbindJSONL(&stamped)
	if err != nil {
		return fmt.Errorf("encoding event: %w", err)
	}
	if err := l.writeDurable(line); err != nil {
		if truncErr := l.file.Truncate(l.size); truncErr != nil {
			l.uncertain = fmt.Errorf("%w: '%s': %w", ErrUncertain, l.path, errors.Join(err, truncErr))
			dl.Errorf("%v", l.uncertain)
			return l.uncertain
		}
		return err
	}
	l.size += int64(len(line))
	l.last = stamped.Seq
	*ev = stamped
	return nil
}

// writeDurable writes line and syncs it. a write and a sync fail the same way
// for the caller: neither is confirmed, and both are rolled back.
func (l *Log) writeDurable(line []byte) error {
	if _, err := l.file.Write(line); err != nil {
		return err
	}
	return l.file.Sync()
}

// LastSeq returns the seq of the newest event, or zero for an empty log.
func (l *Log) LastSeq() int64 {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.last
}

// Since returns every confirmed event with a seq greater than seq, oldest first.
func (l *Log) Since(seq int64) ([]*model.Event, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	var found []*model.Event
	err := scan(l.file, l.size, func(ev *model.Event, _ int64) bool {
		if ev.Seq > seq {
			found = append(found, ev)
		}
		return true
	})
	if err != nil {
		return nil, fmt.Errorf("reading event log '%s': %w", l.path, err)
	}
	return found, nil
}

// Tail returns the newest n events, oldest first.
func (l *Log) Tail(n int) ([]*model.Event, error) {
	if n <= 0 {
		return nil, nil
	}
	l.mu.Lock()
	last := l.last
	l.mu.Unlock()
	return l.Since(max(last-int64(n), 0))
}

// Close closes the underlying file.
func (l *Log) Close() error {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.file.Close()
}
