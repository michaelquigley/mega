package codex

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"testing"

	"git.hq.quigley.com/research/roundhouse/internal/harness"
)

const fakeMode = "ROUNDHOUSE_FAKE_APP_SERVER"

// TestMain lets the test binary stand in for `codex app-server` when the
// fake mode is set, so Launch can be driven through its failure sites.
func TestMain(m *testing.M) {
	if mode := os.Getenv(fakeMode); mode != "" {
		fakeAppServer(mode)
		os.Exit(0)
	}
	os.Exit(m.Run())
}

// fakeAppServer answers initialize, then treats thread/start by mode. frames
// are decoded with encoding/json: test-only stand-in for the other side.
func fakeAppServer(mode string) {
	scanner := bufio.NewScanner(os.Stdin)
	for scanner.Scan() {
		frame := map[string]any{}
		if err := json.Unmarshal(scanner.Bytes(), &frame); err != nil {
			continue
		}
		switch frame["method"] {
		case "initialize":
			fmt.Printf(`{"id":%v,"result":{"userAgent":"fake","codexHome":"/x","platformFamily":"unix","platformOs":"linux"}}`+"\n", frame["id"])
		case "thread/read":
			fmt.Printf(`{"id":%v,"result":{"thread":{"id":"t-1","turns":[`+
				`{"id":"turn-1","status":"completed","items":[{"type":"agentMessage","id":"a-1","text":"done"}]},`+
				`{"id":"turn-2","status":"inProgress","items":[{"type":"userMessage","id":"u-2","content":[{"type":"text","text":"next"}]},{"type":"agentMessage","id":"a-2","text":"going"}]}]}}}`+"\n", frame["id"])
		case "thread/start":
			switch mode {
			case "no-reply":
				os.Exit(0) // as if it died after creating the thread
			case "refuse":
				fmt.Printf(`{"id":%v,"error":{"code":-32000,"message":"refused"}}`+"\n", frame["id"])
			}
		}
	}
}

func fakeHarness(t *testing.T, mode string) *Harness {
	t.Helper()
	t.Setenv(fakeMode, mode)
	return New(Config{Binary: os.Args[0]})
}

func TestNoReplyToThreadStartLeavesTheSessionUnknown(t *testing.T) {
	h := fakeHarness(t, "no-reply")
	_, err := h.Launch(harness.Spec{Cwd: t.TempDir()}, &recordingSink{})
	if err == nil {
		t.Fatal("expected the launch to fail")
	}
	if errors.Is(err, harness.ErrNotStarted) {
		t.Errorf("a thread/start with no reply was classified as not started: %v", err)
	}
}

func TestRefusedThreadStartIsNotStarted(t *testing.T) {
	h := fakeHarness(t, "refuse")
	_, err := h.Launch(harness.Spec{Cwd: t.TempDir()}, &recordingSink{})
	if !errors.Is(err, harness.ErrNotStarted) {
		t.Errorf("an explicit refusal of thread/start: %v", err)
	}
}

func TestMissingAppServerIsNotStarted(t *testing.T) {
	h := New(Config{Binary: "/nonexistent/codex"})
	_, err := h.Launch(harness.Spec{Cwd: t.TempDir()}, &recordingSink{})
	if !errors.Is(err, harness.ErrNotStarted) {
		t.Errorf("a missing app-server: %v", err)
	}
}

func TestHistoryReportsItemsOfATurnInProgressAsOpen(t *testing.T) {
	h := fakeHarness(t, "history")
	history, err := h.History(t.Context(), t.TempDir(), "t-1")
	if err != nil {
		t.Fatal(err)
	}
	if len(history.Events) != 3 {
		t.Fatalf("events = %d", len(history.Events))
	}
	if history.Open["a-1"] || !history.Open["u-2"] || !history.Open["a-2"] {
		t.Errorf("open = %v", history.Open)
	}
}
