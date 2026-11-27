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
	// fakeMCPList is a file holding the mcpServerStatus/list reply, for mode
	// "mcp".
	fakeMCPList = "MEGA_FAKE_APP_SERVER_MCP_LIST"
)

// TestMain lets the test binary stand in for `codex app-server` when the
// fake mode is set, so Launch can be driven.
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
				`{"id":"turn-1","status":"completed","items":[{"type":"agentMessage","id":"a-1","text":"done"},{"type":"commandExecution","id":"c-1","command":"sleep 99","status":"inProgress"}]},`+
				`{"id":"turn-2","status":"inProgress","items":[{"type":"userMessage","id":"u-2","content":[{"type":"text","text":"next"}]},{"type":"agentMessage","id":"a-2","text":"going"}]}]}}}`+"\n", frame["id"])
		case "thread/list":
			fakeThreadList(mode, frame)
		case "model/list":
			fmt.Printf(`{"id":%v,"result":{"data":[`+
				`{"id":"gpt-a","displayName":"A","isDefault":true,"hidden":false,"defaultReasoningEffort":"medium","supportedReasoningEfforts":[{"reasoningEffort":"low"},{"reasoningEffort":"medium"}]},`+
				`{"id":"gpt-b","displayName":"B","isDefault":false,"hidden":false,"defaultReasoningEffort":"low","supportedReasoningEfforts":["low"]},`+
				`{"id":"gpt-hidden","hidden":true,"supportedReasoningEfforts":[]}]}}`+"\n", frame["id"])
		case "config/read":
			fmt.Printf(`{"id":%v,"result":{"config":{"model":"gpt-b","model_reasoning_effort":"low"},"origins":{}}}`+"\n", frame["id"])
		case "turn/start":
			fmt.Printf(`{"id":%v,"result":{"turn":{"id":"turn-%v","status":"inProgress","items":[]}}}`+"\n", frame["id"], frame["id"])
			if mode == "mcp" {
				fmt.Printf(`{"method":"turn/started","params":{"threadId":"t-1","turn":{"id":"turn-%v","status":"inProgress","items":[]}}}`+"\n", frame["id"])
			}
		case "mcpServerStatus/list":
			// the recorded list, for the thread's own id only.
			params, _ := frame["params"].(map[string]any)
			reply, err := os.ReadFile(os.Getenv(fakeMCPList))
			if params["threadId"] != "t-1" || err != nil {
				fmt.Printf(`{"id":%v,"error":{"code":-32602,"message":"no thread"}}`+"\n", frame["id"])
				continue
			}
			fmt.Printf(`{"id":%v,"result":%s}`+"\n", frame["id"], strings.Join(strings.Fields(string(reply)), " "))
		case "thread/start", "thread/resume":
			if mode == "wrong-thread" || mode == "stray" {
				// a frame of another thread, before the reply: in wrong-thread
				// the reply is that other thread too, in stray it is the one
				// asked for.
				fmt.Println(`{"method":"item/completed","params":{"threadId":"t-other","turnId":"turn-9","item":{"type":"agentMessage","id":"x-1","text":"from another thread"}}}`)
				params, _ := frame["params"].(map[string]any)
				thread, _ := params["threadId"].(string)
				if mode == "wrong-thread" {
					thread = "t-other"
				}
				fmt.Printf(`{"id":%v,"result":{"thread":{"id":%q,"cliVersion":"0.166.0"},"model":"gpt-a","reasoningEffort":"high","cwd":"/f"}}`+"\n", frame["id"], thread)
				continue
			}
			if mode == "start" || mode == "mcp" {
				// the thread runs the model passed, or "unset" when none was.
				params, _ := frame["params"].(map[string]any)
				running, ok := params["model"].(string)
				if !ok {
					running = "unset"
				}
				// codex announces each server's startup as the thread starts,
				// before its reply (capability report, 2026-11-12).
				if mode == "mcp" {
					for _, server := range []string{"fixture", "broken"} {
						fmt.Printf(`{"method":"mcpServer/startupStatus/updated","params":{"name":%q,"status":"starting","error":null,"failureReason":null,"threadId":"t-1"}}`+"\n", server)
					}
				}
				fmt.Printf(`{"id":%v,"result":{"thread":{"id":"t-1","cliVersion":"0.166.0"},"model":%q,"reasoningEffort":"high","cwd":"/f"}}`+"\n", frame["id"], running)
				if mode == "mcp" {
					fmt.Println(`{"method":"mcpServer/startupStatus/updated","params":{"name":"fixture","status":"ready","error":null,"failureReason":null,"threadId":"t-1"}}`)
				}
				continue
			}
		}
	}
}

func fakeHarness(t *testing.T, mode string) *Harness {
	t.Helper()
	t.Setenv(fakeMode, mode)
	return New(Config{Binary: os.Args[0]})
}

// a thread read back must be the thread asked for: another is refused before
// anything of it is mapped.
func TestHistoryChecksTheThreadsOwnID(t *testing.T) {
	h := fakeHarness(t, "history")
	if history, err := h.History(t.Context(), harness.Place{Cwd: t.TempDir(), Env: os.Environ(), Find: os.Environ()}, "t-2"); !errors.Is(err, harness.ErrWrongSession) {
		t.Errorf("thread t-1 was read back as t-2: %d events (%v)", len(history.Events), err)
	}
}

func TestHistoryIncludesSpeechAndOmitsCommandsStillRunning(t *testing.T) {
	h := fakeHarness(t, "history")
	history, err := h.History(t.Context(), harness.Place{Cwd: t.TempDir(), Env: os.Environ(), Find: os.Environ()}, "t-1")
	if err != nil {
		t.Fatal(err)
	}
	// a command still running, in a turn that completed, is not imported:
	// only its completion produces a result, under the id it will carry.
	for _, ev := range history.Events {
		if strings.HasPrefix(ev.NativeID, "c-1") {
			t.Errorf("a running command was imported as %s %q", ev.Kind, ev.Text)
		}
	}
	if len(history.Events) != 3 {
		t.Fatalf("events = %d", len(history.Events))
	}
	for i, id := range []string{"a-1", "u-2", "a-2"} {
		if got := history.Events[i].NativeID; got != id {
			t.Errorf("event %d = %q, want %q", i, got, id)
		}
	}
}

func TestCheckNamesAMissingBinary(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	var missing *harness.MissingError
	if err := New(Config{Binary: "codex"}).Check(os.Environ()); !errors.As(err, &missing) || missing.Executable != "codex" || !strings.Contains(err.Error(), "not found on the project's PATH") {
		t.Errorf("no codex: %v", err)
	}
	// a binary that exists, installed whole (helpers_test.go).
	if err := New(Config{Binary: codexPackage(t, t.TempDir(), true)}).Check(os.Environ()); err != nil {
		t.Errorf("a binary that exists: %v", err)
	}
}

// nothing a thread reports reaches the conversation unless it is the thread
// asked for: a frame of another thread, before thread/resume's reply or
// after it, is never mapped, as claude checks every frame's session.
func TestNothingOfAnotherThreadIsRecorded(t *testing.T) {
	for _, mode := range []string{"wrong-thread", "stray"} {
		sink := &recordingSink{}
		rt, err := fakeHarness(t, mode).Launch(harness.Spec{Env: os.Environ(), Find: os.Environ(), Cwd: t.TempDir(), NativeID: "t-1"}, sink)
		if mode == "wrong-thread" && err == nil {
			t.Errorf("%s: a resume that opened another thread launched", mode)
		}
		if rt != nil {
			_ = rt.Close()
		}
		sink.mu.Lock()
		for _, ev := range sink.events {
			if ev.NativeID == "x-1" {
				t.Errorf("%s: another thread's message was recorded: %q", mode, ev.Text)
			}
		}
		sink.mu.Unlock()
	}
}
