package config

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func isolate(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(root, "config"))
	t.Setenv("XDG_DATA_HOME", filepath.Join(root, "data"))
	return root
}

func TestDefaultsFollowXDG(t *testing.T) {
	root := isolate(t)
	cfg, err := Load("", "")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Listen != "127.0.0.1:8470" {
		t.Errorf("listen = %q", cfg.Listen)
	}
	want := filepath.Join(root, "data", "mega")
	if cfg.Data != want {
		t.Errorf("data = %q, want %q", cfg.Data, want)
	}
	if got := cfg.RegistryPath(); got != filepath.Join(want, "registry.yaml") {
		t.Errorf("registry path = %q", got)
	}
	if got := cfg.EventLogPath("abc"); got != filepath.Join(want, "conversations", "abc", "events.jsonl") {
		t.Errorf("event log path = %q", got)
	}
}

func TestConfigFileThenDataFlag(t *testing.T) {
	root := isolate(t)
	dir := filepath.Join(root, "config", "mega")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	body := "listen: 127.0.0.1:9999\ndata: /from/file\n"
	if err := os.WriteFile(filepath.Join(dir, "mega.yaml"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load("", "")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Listen != "127.0.0.1:9999" || cfg.Data != "/from/file" {
		t.Errorf("file not applied: %+v", cfg)
	}
	cfg, err = Load("", "/from/flag")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Data != "/from/flag" {
		t.Errorf("flag did not win: %q", cfg.Data)
	}
}

func TestExplicitConfigMustExist(t *testing.T) {
	isolate(t)
	if _, err := Load(filepath.Join(t.TempDir(), "absent.yaml"), ""); err == nil {
		t.Fatal("expected an error for a missing explicit config")
	}
}

func TestTildeExpansion(t *testing.T) {
	isolate(t)
	t.Setenv("HOME", "/home/someone")
	cfg, err := Load("", "~/elsewhere")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Data != "/home/someone/elsewhere" {
		t.Errorf("data = %q", cfg.Data)
	}
}

func TestListenMustBeLoopback(t *testing.T) {
	for addr, ok := range map[string]bool{
		"127.0.0.1:8470": true,
		"127.1.2.3:8470": true,
		"[::1]:8470":     true,
		"localhost:8470": true,
		"0.0.0.0:8470":   false,
		":8470":          false,
		"192.168.1.5:80": false,
		"[::]:8470":      false,
		"example.com:80": false,
		"not-an-address": false,
	} {
		err := (&Config{Listen: addr}).Validate()
		if ok && err != nil {
			t.Errorf("%q: unexpected error %v", addr, err)
		}
		if !ok && err == nil {
			t.Errorf("%q: expected an error", addr)
		}
	}
}

func TestClaudeDefaultsMergeWithFile(t *testing.T) {
	root := isolate(t)
	t.Setenv("HOME", root)
	path := filepath.Join(t.TempDir(), "mega.yaml")
	if err := os.WriteFile(path, []byte("claude:\n  shim: ~/shim/claude\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(path, "")
	if err != nil {
		t.Fatal(err)
	}
	want := Claude{
		Node:       "node",
		Shim:       filepath.Join(root, "shim", "claude"),
		Executable: filepath.Join(root, ".local", "share", "claude", "versions", "2.1.318"),
	}
	if cfg.Claude != want {
		t.Errorf("claude = %+v, want %+v", cfg.Claude, want)
	}
}

// the default data directory is refused while it is missing and the old
// project's sits beside it, and nothing is created; an explicit one, from the
// flag or the file, is not; nor is the default once moved, or with nothing to
// move.
func TestDefaultDataRefusedUntilTheWorkspaceIsMoved(t *testing.T) {
	root := isolate(t)
	legacy := filepath.Join(root, "data", "roundhouse")
	if err := os.MkdirAll(legacy, 0o755); err != nil {
		t.Fatal(err)
	}
	_, err := Load("", "")
	if !errors.Is(err, ErrNotMoved) || !strings.Contains(err.Error(), "docs/future/mega-cutover.md") || !strings.Contains(err.Error(), legacy) {
		t.Fatalf("err = %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "data", "mega")); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("the refusal created the new data directory: %v", err)
	}

	scratch := filepath.Join(t.TempDir(), "scratch-data")
	if cfg, err := Load("", scratch); err != nil || cfg.Data != scratch {
		t.Errorf("--data: %v %v", cfg, err)
	}
	file := filepath.Join(t.TempDir(), "mega.yaml")
	if err := os.WriteFile(file, []byte("data: "+scratch+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if cfg, err := Load(file, ""); err != nil || cfg.Data != scratch {
		t.Errorf("data from the file: %v %v", cfg, err)
	}

	if err := os.MkdirAll(filepath.Join(root, "data", "mega"), 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := Load("", ""); err != nil {
		t.Errorf("once moved: %v", err)
	}
	if err := os.RemoveAll(filepath.Join(root, "data")); err != nil {
		t.Fatal(err)
	}
	if _, err := Load("", ""); err != nil {
		t.Errorf("nothing to move: %v", err)
	}
}
