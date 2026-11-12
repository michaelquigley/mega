package codex

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/michaelquigley/mega/internal/harness"
)

const (
	fakeMode   = "MEGA_FAKE_APP_SERVER"
	fakeFrames = "MEGA_FAKE_APP_SERVER_FRAMES"
)

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
		if path := os.Getenv(fakeFrames); path != "" {
			// every frame received, for a test to read back.
			if f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600); err == nil {
				_, _ = f.Write(append(scanner.Bytes(), '\n'))
				_ = f.Close()
			}
		}
		frame := map[string]any{}
		if err := json.Unmarshal(scanner.Bytes(), &frame); err != nil {
			continue
		}
		switch frame["method"] {
		case "initialize":
			fmt.Printf(`{"id":%v,"result":{"userAgent":"mega/0.166.0 (fake)","codexHome":"/x","platformFamily":"unix","platformOs":"linux"}}`+"\n", frame["id"])
		case "thread/read":
			fmt.Printf(`{"id":%v,"result":{"thread":{"id":"t-1","turns":[`+
				`{"id":"turn-1","status":"completed","items":[{"type":"agentMessage","id":"a-1","text":"done"}]},`+
				`{"id":"turn-2","status":"inProgress","items":[{"type":"userMessage","id":"u-2","content":[{"type":"text","text":"next"}]},{"type":"agentMessage","id":"a-2","text":"going"}]}]}}}`+"\n", frame["id"])
		case "model/list":
			fmt.Printf(`{"id":%v,"result":{"data":[`+
				`{"id":"gpt-a","displayName":"A","isDefault":true,"hidden":false,"defaultReasoningEffort":"medium","supportedReasoningEfforts":[{"reasoningEffort":"low"},{"reasoningEffort":"medium"}]},`+
				`{"id":"gpt-b","displayName":"B","isDefault":false,"hidden":false,"defaultReasoningEffort":"low","supportedReasoningEfforts":["low"]},`+
				`{"id":"gpt-hidden","hidden":true,"supportedReasoningEfforts":[]}]}}`+"\n", frame["id"])
		case "config/read":
			fmt.Printf(`{"id":%v,"result":{"config":{"model":"gpt-b","model_reasoning_effort":"low"},"origins":{}}}`+"\n", frame["id"])
		case "turn/start":
			fmt.Printf(`{"id":%v,"result":{"turn":{"id":"turn-%v","status":"inProgress","items":[]}}}`+"\n", frame["id"], frame["id"])
		case "thread/start", "thread/resume":
			if mode == "start" {
				// the thread runs the model passed, or "unset" when none was.
				params, _ := frame["params"].(map[string]any)
				running, ok := params["model"].(string)
				if !ok {
					running = "unset"
				}
				fmt.Printf(`{"id":%v,"result":{"thread":{"id":"t-1","cliVersion":"0.166.0"},"model":%q,"reasoningEffort":"high","cwd":"/f"}}`+"\n", frame["id"], running)
				continue
			}
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
	_, err := h.Launch(harness.Spec{Env: os.Environ(), Find: os.Environ(), Cwd: t.TempDir()}, &recordingSink{})
	if err == nil {
		t.Fatal("expected the launch to fail")
	}
	if errors.Is(err, harness.ErrNotStarted) {
		t.Errorf("a thread/start with no reply was classified as not started: %v", err)
	}
}

func TestRefusedThreadStartIsNotStarted(t *testing.T) {
	h := fakeHarness(t, "refuse")
	_, err := h.Launch(harness.Spec{Env: os.Environ(), Find: os.Environ(), Cwd: t.TempDir()}, &recordingSink{})
	if !errors.Is(err, harness.ErrNotStarted) {
		t.Errorf("an explicit refusal of thread/start: %v", err)
	}
}

func TestMissingAppServerIsNotStarted(t *testing.T) {
	h := New(Config{Binary: "/nonexistent/codex"})
	_, err := h.Launch(harness.Spec{Env: os.Environ(), Find: os.Environ(), Cwd: t.TempDir()}, &recordingSink{})
	if !errors.Is(err, harness.ErrNotStarted) {
		t.Errorf("a missing app-server: %v", err)
	}
}

func TestHistoryReportsItemsOfATurnInProgressAsOpen(t *testing.T) {
	h := fakeHarness(t, "history")
	history, err := h.History(t.Context(), harness.Place{Cwd: t.TempDir(), Env: os.Environ(), Find: os.Environ()}, "t-1")
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

func TestCheckNamesAMissingBinary(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	var missing *harness.MissingError
	if err := New(Config{Binary: "codex"}).Check(os.Environ()); !errors.As(err, &missing) || missing.Executable != "codex" || !strings.Contains(err.Error(), "not found on the project's PATH") {
		t.Errorf("no codex: %v", err)
	}
	if err := New(Config{Binary: os.Args[0]}).Check(os.Environ()); err != nil {
		t.Errorf("a binary that exists: %v", err)
	}
}
