package durable

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestMissingStopsAtFirstExisting(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "data", "conversations", "c1")
	got, err := missing(dir)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{dir, filepath.Dir(dir), filepath.Join(root, "data")}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestMkdirAllCreatesWithMode(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "data", "conversations")
	if err := MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(dir)
	if err != nil {
		t.Fatal(err)
	}
	if !info.IsDir() || info.Mode().Perm() != 0o700 {
		t.Errorf("got %v", info.Mode())
	}
	// an existing directory is not an error and creates nothing.
	if err := MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
}
