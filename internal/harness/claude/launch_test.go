package claude

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/michaelquigley/mega/internal/harness"
)

// each claude launch failure comes before or while starting the shim, so each
// is classified as not started; nothing is wrapped as a blanket.
func TestLaunchFailuresBeforeTheShimAreNotStarted(t *testing.T) {
	missingShim := New(Config{Node: "node", Shim: filepath.Join(t.TempDir(), "absent"), Executable: "/x"})
	if _, err := missingShim.Launch(harness.Spec{Env: os.Environ(), Find: os.Environ(), Cwd: t.TempDir()}, &recordingSink{}); !errors.Is(err, harness.ErrNotStarted) {
		t.Errorf("missing shim: %v", err)
	}

	shim := t.TempDir()
	if err := os.WriteFile(filepath.Join(shim, shimScript), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(shim, "node_modules", "@anthropic-ai", "claude-agent-sdk"), 0o700); err != nil {
		t.Fatal(err)
	}
	missingNode := New(Config{Node: filepath.Join(t.TempDir(), "no-node"), Shim: shim, Executable: "/x"})
	if _, err := missingNode.Launch(harness.Spec{Env: os.Environ(), Find: os.Environ(), Cwd: t.TempDir()}, &recordingSink{}); !errors.Is(err, harness.ErrNotStarted) {
		t.Errorf("node that cannot start: %v", err)
	}
}

// Check names node when it cannot be found, and the shim when it is not
// installed.
func TestCheckNamesWhatIsMissing(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	h := New(Config{Node: "node", Shim: t.TempDir(), Executable: "/x"})
	var missing *harness.MissingError
	if err := h.Check(os.Environ()); !errors.As(err, &missing) || missing.Executable != "node" {
		t.Errorf("no node: %v", err)
	}
	node := filepath.Join(t.TempDir(), "node")
	if err := os.WriteFile(node, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	h = New(Config{Node: node, Shim: t.TempDir(), Executable: "/x"})
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
	if err := New(Config{Node: node, Shim: shim, Executable: "/x"}).Check(os.Environ()); err != nil {
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
		if !errors.Is(err, harness.ErrNotStarted) || !errors.As(err, &missing) {
			t.Errorf("%s: launch said %v", name, err)
		}
	}
}
