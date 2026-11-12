package registry

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/michaelquigley/mega/internal/durable"
)

// ErrLocked reports that another process holds the data directory.
var ErrLocked = errors.New("data directory is held by another mega instance")

// Lock is an exclusive hold on a data directory. the port bind only keeps
// same-port instances apart; this keeps a second writer off the registry and
// event logs whatever its listen address. the kernel drops the hold when the
// process exits, so a crash leaves nothing to clean up.
type Lock struct {
	file *os.File
}

// AcquireLock takes the hold on path without waiting, creating the file and
// its directory if needed. the file is never removed, since unlinking a lock
// file another process is about to open would let both believe they hold it.
func AcquireLock(path string) (*Lock, error) {
	// the lock file holds no state, but the data directory this may create does.
	if err := durable.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE, 0o600)
	if err != nil {
		return nil, err
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		defer f.Close()
		if errors.Is(err, syscall.EWOULDBLOCK) {
			// the holder wrote where it serves; empty from an instance that
			// predates the record.
			holder, _ := io.ReadAll(io.LimitReader(f, 512))
			return nil, &LockedError{Dir: filepath.Dir(path), Holder: strings.TrimSpace(string(holder))}
		}
		return nil, fmt.Errorf("locking '%s': %w", path, err)
	}
	return &Lock{file: f}, nil
}

// LockedError is a data directory another process holds, with what that
// process recorded about itself (its address), if anything.
type LockedError struct {
	Dir    string
	Holder string
}

func (e *LockedError) Error() string {
	if e.Holder == "" {
		return fmt.Sprintf("%v: '%s'", ErrLocked, e.Dir)
	}
	return fmt.Sprintf("%v: '%s', serving at '%s'", ErrLocked, e.Dir, e.Holder)
}

func (e *LockedError) Is(target error) bool {
	return target == ErrLocked
}

// Record writes what the holder is, its address, into the lock file, so a
// second instance refused the hold can name it. the file holds nothing else.
func (l *Lock) Record(holder string) error {
	if err := l.file.Truncate(0); err != nil {
		return err
	}
	if _, err := l.file.WriteAt([]byte(holder+"\n"), 0); err != nil {
		return err
	}
	return l.file.Sync()
}

// Release gives the hold up.
func (l *Lock) Release() error {
	return l.file.Close()
}
