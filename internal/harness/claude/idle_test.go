package claude

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/michaelquigley/mega/internal/harness"
	"github.com/michaelquigley/mega/internal/model"
)

// an idle launch asks the shim for ready; a launch with a prompt coming does
// not.
func TestIdleLaunchAsksTheShimForReady(t *testing.T) {
	h, cwd := settingsHarness(t)
	if args := launchArgsFor(t, h, harness.Spec{Env: os.Environ(), Find: os.Environ(), Cwd: cwd, NativeID: "s-1", Idle: true}); !slices.Contains(args, "--idle") {
		t.Errorf("idle launch args = %v", args)
	}
	if args := launchArgsFor(t, h, harness.Spec{Env: os.Environ(), Find: os.Environ(), Cwd: cwd, NativeID: "s-1"}); slices.Contains(args, "--idle") {
		t.Errorf("prompted launch args = %v", args)
	}
}

func modeEvents(evs []*model.Event) []*model.Event {
	var out []*model.Event
	for _, ev := range evs {
		if ev.Kind == model.EventRuntime && ev.Status == model.StatusModeChanged {
			out = append(out, ev)
		}
	}
	return out
}

// ready shows the runtime started with what claude answered and the mode the
// launch asked for, marked requested, and no session, since claude has
// reported none; the first turn's init replaces it with
// claude's report, once, even when it is the same mode.
func TestReadyStartsTheRuntimeWithTheRequestedMode(t *testing.T) {
	r, sink, _ := testRuntime()
	r.launchMode, r.inherited = "acceptEdits", true
	r.handle([]byte(`{"type":"ready","version":"2.1.318","model":"claude-opus-5-5"}`))
	if len(sink.events) != 1 {
		t.Fatalf("events = %v", summary(sink.events))
	}
	started := sink.events[0]
	f := started.Facts
	if started.Status != model.StatusStarted || f.Session != "" || f.Version != "2.1.318" || f.Model != "claude-opus-5-5" || f.PermissionMode != "acceptEdits" || !f.ModeInherited || !f.ModeRequested {
		t.Fatalf("started = %+v %+v", started, f)
	}
	// a second ready, or init at the first turn, starts nothing again.
	r.handle([]byte(`{"type":"ready","version":"x","model":"y"}`))
	init := func(mode, model string) {
		r.handle([]byte(`{"type":"event","msg":{"type":"system","subtype":"init","session_id":"edfe4ade-9f59-4db3-89bc-75fcc8b52c44","permissionMode":"` + mode + `","model":"` + model + `"}}`))
	}
	init("acceptEdits", "claude-opus-5-5")
	init("acceptEdits", "claude-opus-5-5")
	for _, ev := range sink.events[1:] {
		if ev.Status == model.StatusStarted {
			t.Errorf("started twice: %+v", ev)
		}
	}
	reports := modeEvents(sink.events)
	if len(reports) != 1 || reports[0].Facts.PermissionMode != "acceptEdits" || !reports[0].Facts.ModeInherited || reports[0].Facts.ModeRequested || !reports[0].Facts.ModeConfirmed {
		t.Fatalf("reports = %+v", reports)
	}
}

// a first turn reporting another mode than the one requested is a change,
// no longer inherited.
func TestFirstInitAfterReadyReportsADifferentMode(t *testing.T) {
	r, sink, _ := testRuntime()
	r.launchMode, r.inherited = "default", true
	r.handle([]byte(`{"type":"ready","version":"2.1.318","model":"m"}`))
	r.handle([]byte(`{"type":"event","msg":{"type":"system","subtype":"init","session_id":"edfe4ade-9f59-4db3-89bc-75fcc8b52c44","permissionMode":"plan","model":"m"}}`))
	reports := modeEvents(sink.events)
	if len(reports) != 1 || reports[0].Facts.PermissionMode != "plan" || reports[0].Facts.ModeInherited || reports[0].Facts.ModeConfirmed {
		t.Fatalf("reports = %+v", reports)
	}
}

// a switch confirmed before any turn is a report: it replaces the requested
// mode even when it names the same one.
func TestSwitchBeforeAnyTurnReplacesTheRequestedMode(t *testing.T) {
	r, sink, _ := testRuntime()
	r.launchMode = "default"
	r.handle([]byte(`{"type":"ready","version":"2.1.318","model":"m"}`))
	if err := switchWith(r, "default", `{"type":"permission_mode_set","id":"mode-1","mode":"default"}`); err != nil {
		t.Fatal(err)
	}
	reports := modeEvents(sink.events)
	if len(reports) != 1 || reports[0].Facts.PermissionMode != "default" {
		t.Fatalf("reports = %+v", reports)
	}
	// claude's init at the first turn, the same mode: no further report.
	r.handle([]byte(`{"type":"event","msg":{"type":"system","subtype":"init","session_id":"edfe4ade-9f59-4db3-89bc-75fcc8b52c44","permissionMode":"default","model":"m"}}`))
	if n := len(modeEvents(sink.events)); n != 1 {
		t.Errorf("reports after init = %d", n)
	}
}

// a prompted launch is unchanged: the first init starts the runtime with
// claude's own report, and no ready follows.
func TestPromptedLaunchStartsFromInit(t *testing.T) {
	r, sink, _ := testRuntime()
	r.launchMode = "default"
	r.handle([]byte(`{"type":"event","msg":{"type":"system","subtype":"init","session_id":"edfe4ade-9f59-4db3-89bc-75fcc8b52c44","permissionMode":"default","model":"m"}}`))
	if f := sink.events[0].Facts; sink.events[0].Status != model.StatusStarted || f.ModeRequested || f.PermissionMode != "default" {
		t.Fatalf("started = %+v", f)
	}
	if n := len(modeEvents(sink.events)); n != 0 {
		t.Errorf("reports = %d", n)
	}
}

// the first session claude reports in an idle launch is checked, before or
// after ready: the one resumed passes silently; another is reported as the
// runtime's session, for the service to refuse. only the first is checked.
func TestFirstSessionOfAnIdleLaunchIsVerified(t *testing.T) {
	idle := func() (*runtime, *recordingSink) {
		r, sink, _ := testRuntime()
		r.launchMode, r.unverified = "default", true
		return r, sink
	}
	starts := func(sink *recordingSink) []string {
		var out []string
		for _, ev := range sink.events {
			if ev.Status == model.StatusStarted {
				out = append(out, ev.Facts.Session)
			}
		}
		return out
	}
	hook := func(session string) []byte {
		return []byte(`{"type":"event","msg":{"type":"system","subtype":"hook_started","session_id":"` + session + `"}}`)
	}
	ready := []byte(`{"type":"ready","version":"2.1.318","model":"m"}`)

	r, sink := idle()
	r.handle(hook("edfe4ade-9f59-4db3-89bc-75fcc8b52c44"))
	r.handle(ready)
	r.handle([]byte(`{"type":"event","msg":{"type":"system","subtype":"init","session_id":"later","permissionMode":"default","model":"m"}}`))
	if got := starts(sink); len(got) != 1 || got[0] != "" {
		t.Errorf("the resumed session, then another: started %q", got)
	}

	// the checkout's SessionStart hook frames come before ready.
	r, sink = idle()
	r.handle(hook("another"))
	r.handle(ready)
	if got := starts(sink); len(got) != 2 || got[0] != "another" || got[1] != "" {
		t.Errorf("another session before ready: started %q", got)
	}

	r, sink = idle()
	r.handle(ready)
	r.handle([]byte(`{"type":"event","msg":{"type":"system","subtype":"init","session_id":"another","permissionMode":"default","model":"m"}}`))
	r.handle([]byte(`{"type":"event","msg":{"type":"system","subtype":"init","session_id":"yet-another","permissionMode":"default","model":"m"}}`))
	if got := starts(sink); len(got) != 2 || got[0] != "" || got[1] != "another" {
		t.Errorf("another session after ready: started %q", got)
	}

	// a prompted launch is checked by the service at its first init.
	r, sink, _ = testRuntime()
	r.handle(hook("another"))
	if got := starts(sink); len(got) != 0 {
		t.Errorf("a prompted launch's hook frame: started %q", got)
	}
}

// exitSink records a runtime's exit.
type exitSink struct {
	recordingSink
	exited chan error
}

func (s *exitSink) Exited(err error) { s.exited <- err }

// an idle launch that claude never answers is stopped once the timeout
// passes, and reported as an exit with the reason; a launch with a prompt
// coming has no such timeout.
func TestIdleLaunchThatNeverAnswersIsStopped(t *testing.T) {
	saved := idleReadyTimeout
	idleReadyTimeout = 300 * time.Millisecond
	t.Cleanup(func() { idleReadyTimeout = saved })

	shim := t.TempDir()
	if err := os.WriteFile(filepath.Join(shim, shimScript), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(shim, "node_modules", "@anthropic-ai", "claude-agent-sdk"), 0o700); err != nil {
		t.Fatal(err)
	}
	// a stand-in for node that never answers, and starts a child in its
	// group as the shim starts claude.
	node := filepath.Join(t.TempDir(), "node")
	if err := os.WriteFile(node, []byte("#!/bin/sh\nsleep 30 &\nexec sleep 30\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	h := New(Config{Node: node, Shim: shim, Executable: "/x"})

	sink := &exitSink{exited: make(chan error, 1)}
	rt, err := h.Launch(harness.Spec{Env: os.Environ(), Find: os.Environ(), Cwd: t.TempDir(), NativeID: "s-1", Idle: true}, sink)
	if err != nil {
		t.Fatal(err)
	}
	pid := rt.(*runtime).cmd.Process.Pid
	select {
	case err := <-sink.exited:
		if err == nil || !strings.Contains(err.Error(), "did not answer within 300ms of a start with no prompt") {
			t.Errorf("exit = %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("the unanswered idle launch was not stopped")
	}
	// a killed child may linger a moment before it is reaped.
	deadline := time.Now().Add(3 * time.Second)
	for syscall.Kill(-pid, 0) == nil {
		if time.Now().After(deadline) {
			t.Error("the shim's process group outlived the stop")
			break
		}
		time.Sleep(20 * time.Millisecond)
	}

	prompted := &exitSink{exited: make(chan error, 1)}
	rt, err = h.Launch(harness.Spec{Env: os.Environ(), Find: os.Environ(), Cwd: t.TempDir(), NativeID: "s-2"}, prompted)
	if err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-prompted.exited:
		t.Fatalf("a prompted launch was stopped: %v", err)
	case <-time.After(time.Second):
	}
	_ = syscall.Kill(-rt.(*runtime).cmd.Process.Pid, syscall.SIGKILL)
	<-prompted.exited
}
