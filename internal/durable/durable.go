// Package durable makes directory changes survive a machine crash. syncing a
// file protects its bytes, not the directory entries that name it, so anything
// that creates a file or directory holding state syncs its parent as well.
package durable

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// SyncDir flushes dir's entries to disk.
func SyncDir(dir string) error {
	f, err := os.Open(dir)
	if err != nil {
		return err
	}
	defer f.Close()
	return f.Sync()
}

// MkdirAll creates dir and any missing ancestors, as os.MkdirAll does, then
// syncs the parent of each directory it created.
func MkdirAll(dir string, perm os.FileMode) error {
	created, err := missing(dir)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, perm); err != nil {
		return err
	}
	for _, entry := range created {
		if err := SyncDir(filepath.Dir(entry)); err != nil {
			return fmt.Errorf("syncing directory of '%s': %w", entry, err)
		}
	}
	return nil
}

// missing lists path and each of its ancestors that does not exist yet,
// deepest first, stopping at the first that does.
func missing(path string) ([]string, error) {
	var found []string
	for entry := filepath.Clean(path); ; entry = filepath.Dir(entry) {
		_, err := os.Stat(entry)
		if err == nil {
			return found, nil
		}
		if !errors.Is(err, os.ErrNotExist) {
			return nil, err
		}
		found = append(found, entry)
		if parent := filepath.Dir(entry); parent == entry {
			return found, nil
		}
	}
}
