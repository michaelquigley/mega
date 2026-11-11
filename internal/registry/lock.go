package registry

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
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
		f.Close()
		if errors.Is(err, syscall.EWOULDBLOCK) {
			return nil, fmt.Errorf("%w: '%s'", ErrLocked, filepath.Dir(path))
		}
		return nil, fmt.Errorf("locking '%s': %w", path, err)
	}
	return &Lock{file: f}, nil
}

// Release gives the hold up.
func (l *Lock) Release() error {
	return l.file.Close()
}
