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

	"github.com/michaelquigley/mega/internal/durable"
	"github.com/michaelquigley/mega/internal/model"
)

// syncFile and syncDir are variables so a test can fail a sync, which a real
// descriptor does not, and see which directory syncs are made.
var (
	syncFile = (*os.File).Sync
	syncDir  = durable.SyncDir
)

// Log is one conversation's JSONL event history. it has a single writer, the
// service, and assigns each appended event the next seq. size is the confirmed
// length; reads never go past it. the index, built as Open scans the file and
// kept on every append, lets a read seek to where it starts instead of
// parsing from the top.
type Log struct {
	mu     sync.Mutex
	path   string
	file   *os.File // nil until the first append creates the log
	closed bool     // Close has run: nothing is created or appended after it
	size   int64
	last   int64
	index  index
}

// Open opens the log at path. a log that does not exist yet is empty, and is
// created by its first append, so opening one writes nothing. a final line
// with no newline can only be a write torn by a crash, since each append
// writes its newline last, so it is dropped; any other unreadable line is an
// error and is left in place.
func Open(path string) (*Log, error) {
	file, err := os.OpenFile(path, os.O_RDWR|os.O_APPEND, 0)
	if errors.Is(err, os.ErrNotExist) {
		return &Log{path: path}, nil
	}
	if err != nil {
		return nil, err
	}
	info, err := file.Stat()
	if err != nil {
		file.Close()
		return nil, err
	}
	var last, size int64
	var idx index
	err = scan(file, 0, info.Size(), func(ev *model.Event, end int64) bool {
		idx.note(ev, size)
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
	return &Log{path: path, file: file, size: size, last: last, index: idx}, nil
}

// create makes the log's directory and file; the first append syncs the
// entry naming it. callers hold l.mu.
func (l *Log) create() error {
	if err := durable.MkdirAll(filepath.Dir(l.path), 0o700); err != nil {
		return err
	}
	file, err := os.OpenFile(l.path, os.O_RDWR|os.O_CREATE|os.O_APPEND, 0o600)
	if err != nil {
		return err
	}
	l.file = file
	return nil
}

// Remove deletes the log at path together with the directory holding it, which
// Open created for that log alone, and syncs the parent so the deletion
// survives a crash. a log that does not exist is already removed. the caller
// closes any open Log for path first.
func Remove(path string) error {
	dir := filepath.Dir(path)
	if err := os.RemoveAll(dir); err != nil {
		return err
	}
	if err := durable.SyncDir(filepath.Dir(dir)); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("syncing directory of '%s': %w", dir, err)
	}
	return nil
}

// scan reads complete lines of file from offset from up to limit, calling fn
// with each event and the offset just past its newline, until fn returns false
// or the data ends. from must be where a line begins. it reads by offset, so
// it never moves the file's position.
func scan(file *os.File, from, limit int64, fn func(ev *model.Event, end int64) bool) error {
	reader := bufio.NewReader(io.NewSectionReader(file, from, limit-from))
	offset := from
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
			if from > 0 {
				return fmt.Errorf("line %d after offset %d: %w", number, from, err)
			}
			return fmt.Errorf("line %d: %w", number, err)
		}
		if !fn(ev, offset) {
			return nil
		}
	}
}

// Append assigns the next seq, stamps At when unset, and writes the event,
// creating the log if it does not exist yet, and syncing its directory before
// its first event. it returns only once the event is
// on disk. a failure returns its error and leaves the log's seq, its confirmed
// length and ev as they were; the bytes of a failed write may still be on
// disk, and what follows is the caller's to decide.
func (l *Log) Append(ev *model.Event) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.closed {
		return fmt.Errorf("appending to event log '%s': %w", l.path, os.ErrClosed)
	}
	if l.file == nil {
		if err := l.create(); err != nil {
			return err
		}
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
	// a sync covers the file's bytes, not the entry naming it: before the
	// first event the directory is synced too, whether this run created the
	// file or found it empty, so no event lands in a file whose name a crash
	// could still lose.
	if l.size == 0 {
		if err := syncDir(filepath.Dir(l.path)); err != nil {
			return fmt.Errorf("syncing directory of '%s': %w", l.path, err)
		}
	}
	if _, err := l.file.Write(line); err != nil {
		return err
	}
	if err := syncFile(l.file); err != nil {
		return err
	}
	l.index.note(&stamped, l.size)
	l.size += int64(len(line))
	l.last = stamped.Seq
	*ev = stamped
	return nil
}

// LastSeq returns the seq of the newest event, or zero for an empty log.
func (l *Log) LastSeq() int64 {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.last
}

// Since returns every confirmed event with a seq greater than seq, oldest first.
func (l *Log) Since(seq int64) ([]*model.Event, error) {
	return l.Range(seq+1, 0, nil)
}

// Range returns the confirmed events with seqs from from through to, oldest
// first, keeping only those keep accepts when it is not nil. to of zero or
// less reads to the newest event. it seeks to the index's nearest point at or
// before from, so it parses only what it returns and at most one stretch
// between points before it.
func (l *Log) Range(from, to int64, keep func(*model.Event) bool) ([]*model.Event, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if to <= 0 || to > l.last {
		to = l.last
	}
	if from > to || l.file == nil {
		return nil, nil
	}
	var found []*model.Event
	err := scan(l.file, l.index.seek(from), l.size, func(ev *model.Event, _ int64) bool {
		if ev.Seq > to {
			return false
		}
		if ev.Seq >= from && (keep == nil || keep(ev)) {
			found = append(found, ev)
		}
		return true
	})
	if err != nil {
		return nil, fmt.Errorf("reading event log '%s': %w", l.path, err)
	}
	return found, nil
}

// TurnsBefore returns the seq where the n-th turn before seq starts, how many
// turns start before that one, and the newest seq at that moment, so a caller
// can read the stretch as it stood. seq of zero counts back from the end of
// the log, where a newest turn that has not ended is always included and not
// counted among the n; n of zero or less reaches the first event. an empty
// log answers 1 and 0. the turn rule is the index's.
func (l *Log) TurnsBefore(seq int64, n int) (from int64, turnsBefore int, last int64) {
	l.mu.Lock()
	defer l.mu.Unlock()
	from, turnsBefore = l.index.turnsBefore(seq, n)
	return from, turnsBefore, l.last
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

// Close closes the underlying file, if the log has one yet.
func (l *Log) Close() error {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.closed = true
	if l.file == nil {
		return nil
	}
	return l.file.Close()
}
