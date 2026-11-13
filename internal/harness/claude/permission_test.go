package claude

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/michaelquigley/mega/internal/harness"
	"github.com/michaelquigley/mega/internal/model"
)

// settingsHarness is a claude harness whose settings layers all live under
// temporary directories, so no operator settings are read.
func settingsHarness(t *testing.T) (*Harness, string) {
	t.Helper()
	t.Setenv("CLAUDE_CONFIG_DIR", t.TempDir())
	return New(Config{ManagedSettings: filepath.Join(t.TempDir(), "managed-settings.json")}), t.TempDir()
}

func writeSettings(t *testing.T, path, mode string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(`{"permissions":{"defaultMode":"`+mode+`","allow":["Read"]},"theme":"dark"}`), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestSettingsModeFollowsClaudesPrecedence(t *testing.T) {
	h, cwd := settingsHarness(t)
	if mode := h.settingsMode(cwd, os.Environ()); mode != "" {
		t.Fatalf("no settings named a mode, got %q", mode)
	}
	layers := []struct{ path, mode string }{
		{filepath.Join(os.Getenv("CLAUDE_CONFIG_DIR"), "settings.json"), "plan"},
		{filepath.Join(cwd, ".claude", "settings.json"), "acceptEdits"},
		{filepath.Join(cwd, ".claude", "settings.local.json"), "default"},
		{h.cfg.ManagedSettings, "auto"},
	}
	for _, layer := range layers {
		writeSettings(t, layer.path, layer.mode)
		if got := h.settingsMode(cwd, os.Environ()); got != layer.mode {
			t.Errorf("after %s: got %q, want %q", layer.path, got, layer.mode)
		}
	}
}

// nestedCheckout is a git repository with a checkout one level below its root,
// as a project registered on a sub-directory of a repository would be.
func nestedCheckout(t *testing.T) (root, cwd string) {
	t.Helper()
	root = t.TempDir()
	cwd = filepath.Join(root, "service")
	for _, dir := range []string{filepath.Join(root, ".git"), cwd} {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	return root, cwd
}

// claude 2.1.318 reads settings.local.json at the repository root and the
// shared settings.json only in the session's directory (capability report,
// 2026-11-09); the lookup matches it for a checkout below the root.
func TestNestedCheckoutReadsLocalAtTheRootAndSharedInPlace(t *testing.T) {
	h, _ := settingsHarness(t)
	root, cwd := nestedCheckout(t)

	writeSettings(t, filepath.Join(root, ".claude", "settings.json"), "plan")
	if got := h.settingsMode(cwd, os.Environ()); got != "" {
		t.Errorf("the root's shared settings were read from below it: %q", got)
	}
	writeSettings(t, filepath.Join(cwd, ".claude", "settings.json"), "acceptEdits")
	if got := h.settingsMode(cwd, os.Environ()); got != "acceptEdits" {
		t.Errorf("the checkout's own shared settings: %q", got)
	}
	writeSettings(t, filepath.Join(cwd, ".claude", "settings.local.json"), "default")
	if got := h.settingsMode(cwd, os.Environ()); got != "default" {
		t.Errorf("the checkout's own local settings: %q", got)
	}
	writeSettings(t, filepath.Join(root, ".claude", "settings.local.json"), "plan")
	if got := h.settingsMode(cwd, os.Environ()); got != "plan" {
		t.Errorf("the root's local settings win over the checkout's: %q", got)
	}
}

// outside a repository, and in a worktree, the local file stays in the
// session's directory.
func TestLocalSettingsStayInPlaceOutsideARepositoryOrInAWorktree(t *testing.T) {
	h, _ := settingsHarness(t)
	root, cwd := nestedCheckout(t)
	if err := os.Remove(filepath.Join(root, ".git")); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".git"), []byte("gitdir: /elsewhere\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	writeSettings(t, filepath.Join(root, ".claude", "settings.local.json"), "plan")
	if got := h.settingsMode(cwd, os.Environ()); got != "" {
		t.Errorf("a worktree root's local settings were read from below it: %q", got)
	}
	plain := t.TempDir()
	writeSettings(t, filepath.Join(plain, ".claude", "settings.local.json"), "acceptEdits")
	if got := h.settingsMode(plain, os.Environ()); got != "acceptEdits" {
		t.Errorf("outside a repository: %q", got)
	}
}

// claude ignores auto and bypassPermissions from project and local files, so
// mega does too: the settings name no mode, and a new conversation copies
// the manual one.
func TestAutoAndBypassFromProjectFilesAreNotRead(t *testing.T) {
	h, cwd := settingsHarness(t)
	writeSettings(t, filepath.Join(cwd, ".claude", "settings.local.json"), "auto")
	writeSettings(t, filepath.Join(cwd, ".claude", "settings.json"), "bypassPermissions")
	if mode := h.settingsMode(cwd, os.Environ()); mode != "" {
		t.Errorf("settings mode = %q", mode)
	}
	writeSettings(t, filepath.Join(os.Getenv("CLAUDE_CONFIG_DIR"), "settings.json"), "auto")
	if got := h.settingsMode(cwd, os.Environ()); got != "auto" {
		t.Errorf("auto from the user's own settings: %q", got)
	}
}

func TestUnreadableSettingsNameNoMode(t *testing.T) {
	h, cwd := settingsHarness(t)
	path := filepath.Join(cwd, ".claude", "settings.json")
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(`{"permissions":`), 0o600); err != nil {
		t.Fatal(err)
	}
	if mode := h.settingsMode(cwd, os.Environ()); mode != "" {
		t.Errorf("settings mode = %q", mode)
	}
}

// launchArgs launches through a stand-in node that records its arguments and
// exits, and returns them.
func launchArgs(t *testing.T, h *Harness, cwd, mode string) []string {
	t.Helper()
	return launchArgsFor(t, h, harness.Spec{Env: os.Environ(), Find: os.Environ(), Cwd: cwd, PermissionMode: mode})
}

// launchArgsFor is launchArgs for a whole launch spec.
func launchArgsFor(t *testing.T, h *Harness, spec harness.Spec) []string {
	t.Helper()
	shim := t.TempDir()
	if err := os.WriteFile(filepath.Join(shim, shimScript), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(shim, "node_modules", "@anthropic-ai", "claude-agent-sdk"), 0o700); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(t.TempDir(), "args")
	node := filepath.Join(t.TempDir(), "node")
	if err := os.WriteFile(node, []byte("#!/bin/sh\nprintf '%s\\n' \"$@\" > "+out+".tmp && mv "+out+".tmp "+out+"\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	h.cfg.Node, h.cfg.Shim, h.cfg.Executable = node, shim, "/x"
	if _, err := h.Launch(spec, &recordingSink{}); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(5 * time.Second)
	for {
		data, err := os.ReadFile(out)
		if err == nil {
			return strings.Split(strings.TrimSpace(string(data)), "\n")
		}
		if time.Now().After(deadline) {
			t.Fatalf("the stand-in node never ran: %v", err)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func modeArg(args []string) (string, bool) {
	return argValue(args, "--permission-mode")
}

func argValue(args []string, name string) (string, bool) {
	for i, arg := range args {
		if arg == name && i+1 < len(args) {
			return args[i+1], true
		}
	}
	return "", false
}

// the requested mode is always passed: the settings never choose it at
// launch, since a conversation copied them once, at creation. a spec with
// no mode, which the service never sends, passes the manual one.
func TestLaunchAlwaysPassesTheRequestedMode(t *testing.T) {
	h, cwd := settingsHarness(t)
	if mode, ok := modeArg(launchArgs(t, h, cwd, "auto")); !ok || mode != "auto" {
		t.Errorf("requested auto: --permission-mode %q (%v)", mode, ok)
	}
	writeSettings(t, filepath.Join(os.Getenv("CLAUDE_CONFIG_DIR"), "settings.json"), "acceptEdits")
	if mode, ok := modeArg(launchArgs(t, h, cwd, "plan")); !ok || mode != "plan" {
		t.Errorf("the request over the settings: --permission-mode %q (%v)", mode, ok)
	}
	if mode, ok := modeArg(launchArgs(t, h, cwd, "")); !ok || mode != model.PermissionModeDefault {
		t.Errorf("no mode in the spec: --permission-mode %q (%v)", mode, ok)
	}
}

// switchWith switches on another goroutine and plays the shim's reply.
func switchWith(r *runtime, mode, reply string) error {
	result := make(chan error, 1)
	go func() { result <- r.SetPermissionMode(mode) }()
	for {
		r.mu.Lock()
		waiting := len(r.modes) > 0
		r.mu.Unlock()
		if waiting {
			break
		}
		time.Sleep(time.Millisecond)
	}
	r.handle([]byte(reply))
	return <-result
}

func TestSwitchFrameWaitsForTheShimsConfirmation(t *testing.T) {
	r, _, stdin := testRuntime()
	r.mode = "default"
	if err := switchWith(r, "auto", `{"type":"permission_mode_set","id":"mode-1","mode":"auto"}`); err != nil {
		t.Fatal(err)
	}
	if frame := stdin.String(); !strings.Contains(frame, `"type":"set_permission_mode"`) || !strings.Contains(frame, `"id":"mode-1","mode":"auto"`) {
		t.Errorf("frame sent: %s", stdin.String())
	}
	if r.mode != "auto" {
		t.Errorf("mode after a confirmed switch = %q", r.mode)
	}
	err := switchWith(r, "plan", `{"type":"permission_mode_set","id":"mode-2","mode":"plan","error":"not available"}`)
	if err == nil || !strings.Contains(err.Error(), "not available") {
		t.Errorf("refused switch: %v", err)
	}
	if r.mode != "auto" {
		t.Errorf("a refused switch moved the mode to %q", r.mode)
	}
	r.gone = true
	if err := r.SetPermissionMode("default"); !errors.Is(err, errExited) {
		t.Errorf("switch after exit: %v", err)
	}
}

func TestInitAndRequestsCarryTheMode(t *testing.T) {
	r, sink, _ := testRuntime()
	init := func(mode string) {
		r.handle([]byte(`{"type":"event","msg":{"type":"system","subtype":"init","session_id":"edfe4ade-9f59-4db3-89bc-75fcc8b52c44","permissionMode":"` + mode + `"}}`))
	}
	init("plan")
	started := sink.events[0]
	if started.Status != model.StatusStarted || started.Facts.PermissionMode != "plan" {
		t.Fatalf("started = %+v %+v", started, started.Facts)
	}
	init("plan") // every turn re-reports init; an unchanged mode is not a change
	init("default")
	var changes []string
	for _, ev := range sink.events {
		if ev.Kind == model.EventRuntime && ev.Status == model.StatusModeChanged {
			changes = append(changes, ev.Facts.PermissionMode)
		}
	}
	if strings.Join(changes, ",") != "default" {
		t.Errorf("mode changes reported = %v", changes)
	}

	r.mode = "auto"
	r.handle([]byte(`{"type":"request","id":"sdk-1","kind":"permission","tool":"Bash","input":{"command":"sleep 3"}}`))
	if payload := sink.payloads[0].(map[string]any); payload["permission_mode"] != "auto" {
		t.Errorf("request payload = %v", payload)
	}
}

func TestTimedOutSwitchIsUnconfirmedAndALateConfirmationIsReported(t *testing.T) {
	r, sink, _ := testRuntime()
	r.mode = "default"
	saved := modeTimeout
	modeTimeout = 20 * time.Millisecond
	defer func() { modeTimeout = saved }()

	err := r.SetPermissionMode("auto")
	if !errors.Is(err, harness.ErrModeUnconfirmed) || r.mode != "default" {
		t.Fatalf("timed out: %v, mode %q", err, r.mode)
	}
	r.handle([]byte(`{"type":"permission_mode_set","id":"mode-1","mode":"auto"}`))
	if r.mode != "auto" || len(sink.events) != 1 {
		t.Fatalf("after the late confirmation: mode %q, events %d", r.mode, len(sink.events))
	}
	if ev := sink.events[0]; ev.Status != model.StatusModeChanged || ev.Facts.PermissionMode != "auto" {
		t.Errorf("late confirmation reported as %+v", ev)
	}

	if err := r.SetPermissionMode("plan"); !errors.Is(err, harness.ErrModeUnconfirmed) {
		t.Fatal(err)
	}
	r.handle([]byte(`{"type":"permission_mode_set","id":"mode-2","mode":"plan","error":"not available"}`))
	if r.mode != "auto" || len(sink.events) != 1 {
		t.Errorf("a late refusal moved the mode to %q or reported %d events", r.mode, len(sink.events))
	}
}

// the reader applies a confirmed mode as it reads the confirmation: a request
// it reads next carries the new mode, and the waiter, whenever it resumes,
// never overwrites a newer reading such as the next init.
func TestConfirmedModeAppliesWhenTheReplyIsRead(t *testing.T) {
	r, sink, _ := testRuntime()
	r.mode = "default"
	r.started = true
	result := make(chan error, 1)
	go func() { result <- r.SetPermissionMode("auto") }()
	for {
		r.mu.Lock()
		waiting := len(r.modes) > 0
		r.mu.Unlock()
		if waiting {
			break
		}
		time.Sleep(time.Millisecond)
	}
	r.handle([]byte(`{"type":"permission_mode_set","id":"mode-1","mode":"auto"}`))
	r.handle([]byte(`{"type":"request","id":"sdk-1","kind":"permission","tool":"Bash","input":{"command":"sleep 3"}}`))
	r.handle([]byte(`{"type":"event","msg":{"type":"system","subtype":"init","session_id":"edfe4ade-9f59-4db3-89bc-75fcc8b52c44","permissionMode":"plan"}}`))
	if err := <-result; err != nil {
		t.Fatal(err)
	}
	if payload := sink.payloads[0].(map[string]any); payload["permission_mode"] != "auto" {
		t.Errorf("the request read after the confirmation carries %v", payload["permission_mode"])
	}
	r.mu.Lock()
	mode := r.mode
	r.mu.Unlock()
	if mode != "plan" {
		t.Errorf("mode after the switch returned = %q, want the newer init's plan", mode)
	}
}
