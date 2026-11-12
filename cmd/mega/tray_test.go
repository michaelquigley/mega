package main

import (
	"bytes"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/michaelquigley/mega/internal/harness"
	"github.com/michaelquigley/mega/internal/harness/harnesstest"
	"github.com/michaelquigley/mega/internal/model"
	"github.com/michaelquigley/mega/internal/registry"
	"github.com/michaelquigley/mega/internal/service"
)

// fakeTray records what the daemon asks of its tray.
type fakeTray struct {
	mu       sync.Mutex
	icons    [][]byte
	tooltips []string
	labels   map[int][]string
	stops    int
}

func (f *fakeTray) SetIcon(png []byte) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.icons = append(f.icons, png)
	return nil
}
func (f *fakeTray) SetTooltip(tooltip string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.tooltips = append(f.tooltips, tooltip)
}
func (f *fakeTray) SetItemLabel(index int, label string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.labels == nil {
		f.labels = map[int][]string{}
	}
	f.labels[index] = append(f.labels[index], label)
}
func (f *fakeTray) Stop() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.stops++
}

func (f *fakeTray) last() (icon []byte, tooltip, gateLabel string, calls int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.icons) > 0 {
		icon = f.icons[len(f.icons)-1]
	}
	if len(f.tooltips) > 0 {
		tooltip = f.tooltips[len(f.tooltips)-1]
	}
	if l := f.labels[itemGate]; len(l) > 0 {
		gateLabel = l[len(l)-1]
	}
	return icon, tooltip, gateLabel, len(f.icons) + len(f.tooltips) + len(f.labels[itemGate])
}

// a service over a fake harness, with one project, and its conversation idle
// on a warm runtime.
func trayService(t *testing.T) (*service.Service, *harnesstest.Harness, model.Project, service.ConversationView) {
	t.Helper()
	dir := t.TempDir()
	reg, err := registry.Load(filepath.Join(dir, "registry.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	fake := harnesstest.New(model.HarnessClaude)
	svc, err := service.New(reg, service.Options{
		EventLogPath: func(id string) string { return filepath.Join(dir, "conversations", id, "events.jsonl") },
		Harnesses:    []harness.Harness{fake},
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(svc.Close)
	project, err := svc.RegisterProject("p", t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	conv, err := svc.StartConversation(project.ID, model.HarnessClaude, "", "one", service.LaunchChoice{})
	if err != nil {
		t.Fatal(err)
	}
	rt := fake.Last()
	rt.Emit(&model.Event{Kind: model.EventRuntime, Status: model.StatusStarted, Facts: &model.RuntimeFacts{Session: rt.NativeID()}})
	rt.Emit(&model.Event{Kind: model.EventAssistant, Text: "done", NativeID: "a-1"})
	rt.Emit(&model.Event{Kind: model.EventTurnEnd, Status: model.TurnSuccess})
	if err := svc.Read(conv.ID, svc.Workspace().Conversations[0].LastSeq); err != nil {
		t.Fatal(err)
	}
	return svc, fake, project, conv
}

func requested(svc *service.Service) bool {
	select {
	case <-svc.ShutdownRequested():
		return true
	default:
		return false
	}
}

// Quit on an idle workspace begins the orderly stop and opens nothing; dfw is
// told not to quit, so the page is served until the stop is done.
func TestQuitOnAnIdleWorkspaceShutsDown(t *testing.T) {
	svc, _, _, _ := trayService(t)
	opened := 0
	actions := trayActions{gate: svc, open: func() { opened++ }}
	if actions.onQuit() {
		t.Error("Quit let dfw quit before the orderly stop")
	}
	if !requested(svc) || opened != 0 {
		t.Errorf("idle: shutdown requested %v, page opened %d", requested(svc), opened)
	}
}

// Quit with anything in flight arms the gate and opens the page; nothing
// stops, nothing is answered or interrupted.
func TestQuitWhileBusyArmsTheGate(t *testing.T) {
	svc, fake, _, conv := trayService(t)
	if err := svc.Prompt(conv.ID, "busy"); err != nil {
		t.Fatal(err)
	}
	rt := fake.Last()
	opened := 0
	actions := trayActions{gate: svc, open: func() { opened++ }}
	if actions.onQuit() {
		t.Error("Quit let dfw quit with a turn in flight")
	}
	if requested(svc) || !svc.Workspace().Gate.Armed || opened != 1 {
		t.Errorf("busy: shutdown requested %v, armed %v, page opened %d", requested(svc), svc.Workspace().Gate.Armed, opened)
	}
	if rt.Interrupts() != 0 || rt.Closed() {
		t.Error("Quit interrupted or released the runtime")
	}
	// the turn ends; a second Quit is the orderly stop.
	rt.Emit(&model.Event{Kind: model.EventAssistant, Text: "done", NativeID: "a-2"})
	rt.Emit(&model.Event{Kind: model.EventTurnEnd, Status: model.TurnSuccess})
	if actions.onQuit() || !requested(svc) {
		t.Errorf("idle at last: shutdown requested %v", requested(svc))
	}
}

// the gate item arms and opens the page, and while armed cancels.
func TestGateItemArmsAndCancels(t *testing.T) {
	svc, _, _, _ := trayService(t)
	opened := 0
	actions := trayActions{gate: svc, open: func() { opened++ }}
	actions.onGateItem()
	if !svc.Workspace().Gate.Armed || opened != 1 || requested(svc) {
		t.Fatalf("arm: armed %v, opened %d", svc.Workspace().Gate.Armed, opened)
	}
	actions.onGateItem()
	if svc.Workspace().Gate.Armed || opened != 1 {
		t.Errorf("cancel: armed %v, opened %d", svc.Workspace().Gate.Armed, opened)
	}
}

// the tray follows the needs-you list and the gate through the service's own
// updates: the needs-you icon and a count while anything waits, the mark and
// "mega" otherwise; the gate item's label with the gate; nothing re-sent when
// nothing changed.
func TestTrayFollowsNeedsYouAndTheGate(t *testing.T) {
	svc, fake, _, conv := trayService(t)
	ui := &fakeTray{}
	follower := &trayFollower{ui: ui}
	done := make(chan struct{})
	finished := make(chan struct{})
	go func() {
		follower.follow(svc, done)
		close(finished)
	}()
	wait := func(what string, icon []byte, tooltip, label string) {
		t.Helper()
		deadline := time.Now().Add(2 * time.Second)
		for {
			gotIcon, gotTooltip, gotLabel, _ := ui.last()
			if bytes.Equal(gotIcon, icon) && gotTooltip == tooltip && gotLabel == label {
				return
			}
			if time.Now().After(deadline) {
				t.Fatalf("%s: tooltip %q, label %q, needs-you icon %v", what, gotTooltip, gotLabel, bytes.Equal(gotIcon, trayIconNeedsYouPNG))
			}
			time.Sleep(5 * time.Millisecond)
		}
	}
	wait("nothing waiting", trayIconPNG, "mega", gateArmLabel)
	_, _, _, before := ui.last()
	svc.Workspace() // reads change nothing on the tray.
	if _, _, _, after := ui.last(); after != before {
		t.Error("a read re-sent the tray")
	}

	if err := svc.Prompt(conv.ID, "ask"); err != nil {
		t.Fatal(err)
	}
	rt := fake.Last()
	request := rt.Ask(model.RequestPermission, map[string]any{"tool": "Bash"})
	wait("a request pending", trayIconNeedsYouPNG, "mega: 1 needs you", gateArmLabel)

	if err := svc.ArmShutdown(); err != nil {
		t.Fatal(err)
	}
	wait("armed", trayIconNeedsYouPNG, "mega: 1 needs you", gateCancelLabel)
	if err := svc.CancelShutdown(); err != nil {
		t.Fatal(err)
	}

	if err := svc.Answer(conv.ID, request, harness.Answer{Decision: "allow"}); err != nil {
		t.Fatal(err)
	}
	rt.Emit(&model.Event{Kind: model.EventAssistant, Text: "allowed", NativeID: "a-3"})
	rt.Emit(&model.Event{Kind: model.EventTurnEnd, Status: model.TurnSuccess})
	// answered, the reply is unread: it still needs the operator.
	wait("answered, unread", trayIconNeedsYouPNG, "mega: 1 needs you", gateArmLabel)
	ws := svc.Workspace()
	if err := svc.Read(conv.ID, ws.Conversations[0].LastSeq); err != nil {
		t.Fatal(err)
	}
	wait("read", trayIconPNG, "mega", gateArmLabel)

	close(done)
	svc.Close()
	select {
	case <-finished:
	case <-time.After(2 * time.Second):
		t.Fatal("follow did not end with the service")
	}
}

func TestTrayStateWording(t *testing.T) {
	for _, c := range []struct {
		n    int
		want string
	}{{0, "mega"}, {1, "mega: 1 needs you"}, {3, "mega: 3 need you"}} {
		ws := service.Workspace{NeedsYou: make([]string, c.n)}
		if got := stateOf(ws).tooltip; got != c.want {
			t.Errorf("%d waiting: %q, want %q", c.n, got, c.want)
		}
	}
}
