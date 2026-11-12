package harness

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFindExecutable(t *testing.T) {
	dir := t.TempDir()
	tool := filepath.Join(dir, "tool")
	if err := os.WriteFile(tool, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	plain := filepath.Join(dir, "plain")
	if err := os.WriteFile(plain, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir)
	for _, ok := range []string{"tool", tool} {
		if err := FindExecutable("h", ok, os.Environ()); err != nil {
			t.Errorf("%s: %v", ok, err)
		}
	}
	for name, reason := range map[string]string{
		"absent":                   "'absent' was not found on the project's PATH",
		filepath.Join(dir, "gone"): "does not exist",
		plain:                      "is not an executable file",
		dir:                        "is not an executable file",
		"":                         "no executable is configured",
	} {
		err := FindExecutable("h", name, os.Environ())
		var missing *MissingError
		if !errors.As(err, &missing) || !errors.Is(err, ErrExecutableMissing) || !strings.Contains(err.Error(), reason) || missing.Executable != name {
			t.Errorf("%q: %v", name, err)
		}
	}
}
