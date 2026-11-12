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

// the holder's record names it to a second instance refused the hold; a
// holder that recorded nothing is still refused, without a name.
func TestLockedErrorNamesTheHolder(t *testing.T) {
	path := filepath.Join(t.TempDir(), "data", "lock")
	first, err := AcquireLock(path)
	if err != nil {
		t.Fatal(err)
	}
	defer first.Release()
	_, err = AcquireLock(path)
	var held *LockedError
	if !errors.As(err, &held) || held.Holder != "" {
		t.Fatalf("before a record: %v", err)
	}
	if err := first.Record("http://127.0.0.1:18470"); err != nil {
		t.Fatal(err)
	}
	if err := first.Record("http://127.0.0.1:8470"); err != nil {
		t.Fatal(err)
	}
	_, err = AcquireLock(path)
	if !errors.As(err, &held) || held.Holder != "http://127.0.0.1:8470" || !errors.Is(err, ErrLocked) {
		t.Fatalf("after a record: %v", err)
	}
}
