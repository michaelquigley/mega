package claude

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/michaelquigley/mega/internal/harness"
)

// Check names node when it cannot be found, and the shim when it is not
// installed.
func TestCheckNamesWhatIsMissing(t *testing.T) {
	trueBin := truePath(t) // before the PATH is emptied
	t.Setenv("PATH", t.TempDir())
	h := New(Config{Node: "node", Shim: t.TempDir(), Executable: trueBin})
	var missing *harness.MissingError
	if err := h.Check(os.Environ()); !errors.As(err, &missing) || missing.Executable != "node" {
		t.Errorf("no node: %v", err)
	}
	node := filepath.Join(t.TempDir(), "node")
	if err := os.WriteFile(node, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	h = New(Config{Node: node, Shim: t.TempDir(), Executable: trueBin})
	if err := h.Check(os.Environ()); !errors.As(err, &missing) || missing.Executable != "the claude shim" {
		t.Errorf("no shim: %v", err)
	}
	shim := t.TempDir()
	if err := os.WriteFile(filepath.Join(shim, shimScript), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(shim, "node_modules", "@anthropic-ai", "claude-agent-sdk"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := New(Config{Node: node, Shim: shim, Executable: trueBin}).Check(os.Environ()); err != nil {
		t.Errorf("all present: %v", err)
	}
}

// a claude executable that is a script is refused, in Check and at launch:
// the sdk would start it through an interpreter found on the activated PATH.
func TestAScriptClaudeExecutableIsRefused(t *testing.T) {
	shim := t.TempDir()
	if err := os.WriteFile(filepath.Join(shim, shimScript), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(shim, "node_modules", "@anthropic-ai", "claude-agent-sdk"), 0o700); err != nil {
		t.Fatal(err)
	}
	node := filepath.Join(t.TempDir(), "node")
	if err := os.WriteFile(node, []byte("\x7fELF stand-in\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	for name, body := range map[string]string{
		"claude":    "#!/usr/bin/env node\nrequire('./cli.js')\n",
		"claude.js": "// no #! line, but javascript all the same\n",
	} {
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, []byte(body), 0o755); err != nil {
			t.Fatal(err)
		}
		h := New(Config{Node: node, Shim: shim, Executable: path})
		var missing *harness.MissingError
		if err := h.Check(os.Environ()); !errors.As(err, &missing) || !strings.Contains(err.Error(), "native build") {
			t.Errorf("%s: Check said %v", name, err)
		}
		_, err := h.Launch(harness.Spec{Env: os.Environ(), Find: os.Environ(), Cwd: t.TempDir()}, &recordingSink{})
		if !errors.As(err, &missing) {
			t.Errorf("%s: launch said %v", name, err)
		}
	}
}

// a configured claude code path that does not exist, or is not an
// executable file, is named before anything runs, with what to do and where
// it is set; the sdk is never left to fail on it.
func TestAMissingClaudeCodeIsNamed(t *testing.T) {
	shim := t.TempDir()
	if err := os.WriteFile(filepath.Join(shim, shimScript), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(shim, "node_modules", "@anthropic-ai", "claude-agent-sdk"), 0o700); err != nil {
		t.Fatal(err)
	}
	where := "in '/c/mega.yaml', the config file mega read"
	gone := filepath.Join(t.TempDir(), "versions", "2.1.318")
	plain := filepath.Join(t.TempDir(), "claude")
	if err := os.WriteFile(plain, []byte("not a program"), 0o644); err != nil {
		t.Fatal(err)
	}
	for path, what := range map[string]string{gone: "does not exist", plain: "is not an executable file"} {
		h := New(Config{Node: "node", Shim: shim, Executable: path, Where: where})
		err := h.Check(os.Environ())
		var missing *harness.MissingError
		if !errors.As(err, &missing) || missing.Executable != path {
			t.Fatalf("%s: %v", path, err)
		}
		want := fmt.Sprintf("'claude' cannot run: '%s' %s; install Claude Code, or set claude.executable %s, to where it is", path, what, where)
		if err.Error() != want {
			t.Errorf("got  %q\nwant %q", err.Error(), want)
		}
		if _, err := h.Models(t.Context(), harness.Place{Cwd: t.TempDir(), Env: os.Environ(), Find: os.Environ()}); !errors.As(err, &missing) {
			t.Errorf("%s: models ran past a missing claude code: %v", path, err)
		}
	}
}
