package registry

import (
	"errors"
	"path/filepath"
	"testing"
)

func TestLockExcludesSecondHolder(t *testing.T) {
	path := filepath.Join(t.TempDir(), "data", "lock")
	first, err := AcquireLock(path)
	if err != nil {
		t.Fatal(err)
	}

	// flock holds are per open file description, so a second open in the same
	// process contends exactly as a second process would.
	if _, err := AcquireLock(path); !errors.Is(err, ErrLocked) {
		t.Fatalf("expected ErrLocked, got %v", err)
	}

	if err := first.Release(); err != nil {
		t.Fatal(err)
	}
	second, err := AcquireLock(path)
	if err != nil {
		t.Fatalf("lock not reacquirable after release: %v", err)
	}
	second.Release()
}
