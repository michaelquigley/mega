package claude

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/michaelquigley/mega/internal/harness"
	"github.com/michaelquigley/mega/internal/model"
)

// recorded is a harness whose fake shim serves the recorded session file as
// the messages of the session asked, and the session id the recording's
// messages carry.
func recorded(t *testing.T, file, asked string) *Harness {
	t.Helper()
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("no node")
	}
	shim := t.TempDir()
	fake, err := filepath.Abs(filepath.Join("..", "..", "..", "shim", "claude", "fake", "shim.mjs"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(fake, filepath.Join(shim, "shim.mjs")); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(shim, "node_modules", "@anthropic-ai", "claude-agent-sdk"), 0o700); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join("testdata", file))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(shim, "messages-"+asked+".jsonl"), data, 0o600); err != nil {
		t.Fatal(err)
	}
	return New(Config{Node: node, Shim: shim, Executable: truePath(t)})
}

// sessionOf is the session id a recording's messages carry.
func sessionOf(t *testing.T, file string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", file))
	if err != nil {
		t.Fatal(err)
	}
	first, _, _ := bytes.Cut(data, []byte("\n"))
	var m struct {
		SessionID string `json:"session_id"`
	}
	if err := json.Unmarshal(first, &m); err != nil || m.SessionID == "" {
		t.Fatalf("no session id in %s: %v", file, err)
	}
	return m.SessionID
}

// a look at a recorded session, through the shim's messages command as an
// import reads it (here the fake shim's, serving the recorded transcript):
// the first prompt, the last, and the last reply.
func TestLookReadsARecordedSession(t *testing.T) {
	id := sessionOf(t, "history.jsonl")
	h := recorded(t, "history.jsonl", id)
	look, err := h.Look(t.Context(), harness.Place{Cwd: t.TempDir(), Env: os.Environ(), Find: os.Environ()}, id)
	if err != nil {
		t.Fatal(err)
	}
	if look.FirstPrompt != "Reply with the project instruction marker followed by ALPHA-1a2b" || look.LastPrompt != "Reply with TERMINAL-MARKER-e9d1" || look.LastReply != "TERMINAL-MARKER-e9d1" || look.Prompts != 2 {
		t.Errorf("look: %+v", look)
	}
}

// a model switch's /model record, read back through Harness.History and the
// shim's messages command (here the fake shim's, serving it as the sdk gave
// it, marked isCompletedLocalCommand; recorded 2026-11-19 from the
// claude-rt1 fixture, claude code 2.1.326), beside a prompt made only of the
// same tags and unmarked: the record is native, the prompt the operator's.
func TestHistoryKeepsAModelSwitchRecordNative(t *testing.T) {
	id := sessionOf(t, "model-switch-history.jsonl")
	h := recorded(t, "model-switch-history.jsonl", id)
	history, err := h.History(t.Context(), harness.Place{Cwd: t.TempDir(), Env: os.Environ(), Find: os.Environ()}, id)
	if err != nil {
		t.Fatal(err)
	}
	evs := history.Events
	if len(evs) != 2 {
		t.Fatalf("history = %+v", evs)
	}
	if evs[0].Kind != model.EventNative || evs[0].Text != "user/local-command" || evs[0].NativeID != "b1f8d962-ce9a-4033-b49a-8fa15c9d0f9b" {
		t.Errorf("the /model record imports as %+v", evs[0])
	}
	if evs[1].Kind != model.EventUser || evs[1].Text != "<command-args>Explain why this failed.</command-args>" {
		t.Errorf("the prompt made of tags imports as %+v", evs[1])
	}
}

// historyFrom reads a recorded session through Harness.History and the fake
// shim's messages command, as an import does.
func historyFrom(t *testing.T, file string) harness.History {
	t.Helper()
	id := sessionOf(t, file)
	h := recorded(t, file, id)
	history, err := h.History(t.Context(), harness.Place{Cwd: t.TempDir(), Env: os.Environ(), Find: os.Environ()}, id)
	if err != nil {
		t.Fatal(err)
	}
	return history
}

func TestHistoryIncludesTextFromAPromptWithAnAttachment(t *testing.T) {
	history := historyFrom(t, "attachment-history.jsonl")
	if len(history.Events) != 2 || history.Events[0].Kind != model.EventUser || history.Events[0].Text != "Look at this attachment." || history.Events[1].Kind != model.EventNative {
		t.Fatalf("events = %+v", history.Events)
	}
}

// a history read back must be the session asked for, message by message, as
// the live stream is checked frame by frame: messages of another session are
// refused before anything of them is imported.
func TestHistoryRefusesAnotherSessionsMessages(t *testing.T) {
	h := recorded(t, "history.jsonl", "not-this-one")
	history, err := h.History(t.Context(), harness.Place{Cwd: t.TempDir(), Env: os.Environ(), Find: os.Environ()}, "not-this-one")
	if !errors.Is(err, harness.ErrWrongSession) {
		t.Errorf("another session's messages were read back: %d events (%v)", len(history.Events), err)
	}
}
