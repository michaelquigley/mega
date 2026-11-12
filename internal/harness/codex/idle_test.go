package codex

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/michaelquigley/mega/internal/harness"
	"github.com/michaelquigley/mega/internal/model"
)

// an idle launch resumes the exact thread and starts no turn: thread/resume
// answers with what codex runs, which is reported as started at once, and
// nothing more is sent until a prompt.
func TestIdleLaunchResumesTheThreadAndStartsNoTurn(t *testing.T) {
	h := fakeHarness(t, "start")
	frames := filepath.Join(t.TempDir(), "frames")
	t.Setenv(fakeFrames, frames)
	sink := &recordingSink{}
	rt, err := h.Launch(harness.Spec{Env: os.Environ(), Cwd: t.TempDir(), NativeID: "t-1", Idle: true}, sink)
	if err != nil {
		t.Fatal(err)
	}
	if rt.NativeID() != "t-1" {
		t.Errorf("native id = %q", rt.NativeID())
	}
	sink.mu.Lock()
	evs := append([]*model.Event(nil), sink.events...)
	sink.mu.Unlock()
	if len(evs) != 1 || evs[0].Status != model.StatusStarted || evs[0].Facts.Session != "t-1" || evs[0].Facts.Version != "0.166.0" || evs[0].Facts.Effort != "high" {
		t.Fatalf("events = %v", summary(evs))
	}
	if err := rt.Close(); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(frames)
	if err != nil {
		t.Fatal(err)
	}
	var methods []string
	for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		frame := map[string]any{}
		if err := json.Unmarshal([]byte(line), &frame); err != nil {
			t.Fatal(err)
		}
		if method, ok := frame["method"].(string); ok && method != "initialized" {
			methods = append(methods, method)
		}
	}
	for _, method := range methods {
		if method == "turn/start" || method == "thread/start" {
			t.Errorf("an idle resume sent %s: %v", method, methods)
		}
	}
	if !strings.Contains(strings.Join(methods, ","), "thread/resume") {
		t.Errorf("sent %v, want a thread/resume", methods)
	}
}
