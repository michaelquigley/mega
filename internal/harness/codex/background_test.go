package codex

import (
	"bufio"
	"bytes"
	"encoding/json"
	"os"
	"strings"
	"testing"

	"git.hq.quigley.com/research/roundhouse/internal/model"
)

// recorded from codex-cli 0.166.0 (probes/codex/background.mjs --session): a
// command started as a terminal session outlives its turn. once the turn
// completes it is background work; its item/completed arrives after the turn
// and is one native event, opening no turn, with the list emptied.
func TestCommandOutlivingItsTurnIsBackgroundUntilItCompletes(t *testing.T) {
	r, sink, _ := testRuntime()
	data, err := os.ReadFile("testdata/background.jsonl")
	if err != nil {
		t.Fatal(err)
	}
	var completedAt int
	scanner := bufio.NewScanner(bytes.NewReader(data))
	scanner.Buffer(make([]byte, 1<<20), 1<<20)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.Contains(line, `"turn/completed"`) {
			completedAt = len(sink.events)
		}
		r.feed(line)
	}
	if len(sink.background) != 2 {
		t.Fatalf("lists reported = %+v", sink.background)
	}
	running, done := sink.background[0], sink.background[1]
	if len(running) != 1 || running[0].Description != "sleep 30 && echo BG-CODEX-DONE" || running[0].Kind != "command" || running[0].StartedAt.IsZero() {
		t.Errorf("at the turn's end: %+v", running)
	}
	if len(done) != 0 {
		t.Errorf("after the command completed: %+v", done)
	}
	// after the turn's end: the command's call and result, marked as after
	// their turn, with the identities a history import gives the same item.
	after := sink.events[completedAt+1:]
	if len(after) != 2 || after[0].Kind != model.EventToolUse || after[1].Kind != model.EventToolResult || !after[0].AfterTurn || !after[1].AfterTurn {
		t.Fatalf("events after the turn ended = %v", summary(after))
	}
	if after[1].Text != "BG-CODEX-DONE\n" {
		t.Errorf("the result = %q", after[1].Text)
	}
	var completed map[string]any
	for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		if strings.Contains(line, `"item/completed"`) && strings.Contains(line, "commandExecution") {
			frame := map[string]any{}
			if err := json.Unmarshal([]byte(line), &frame); err != nil {
				t.Fatal(err)
			}
			completed = frame["params"].(map[string]any)["item"].(map[string]any)
		}
	}
	imported, err := mapItem(completed)
	if err != nil {
		t.Fatal(err)
	}
	if len(imported) != 2 || imported[0].NativeID != after[0].NativeID || imported[1].NativeID != after[1].NativeID {
		t.Errorf("live ids %q %q, import ids %v", after[0].NativeID, after[1].NativeID, []string{imported[0].NativeID, imported[1].NativeID})
	}
}

// a command that completes inside its own turn is an ordinary tool call, and
// never background.
func TestCommandCompletingInItsTurnIsNotBackground(t *testing.T) {
	r, sink, _ := testRuntime()
	r.feed(`{"method":"turn/started","params":{"threadId":"thread-1","turn":{"id":"t1","status":"inProgress","items":[]}}}`)
	r.feed(`{"method":"item/started","params":{"threadId":"thread-1","turnId":"t1","item":{"type":"commandExecution","id":"c1","command":"/bin/bash -lc 'ls'","status":"inProgress"}}}`)
	r.feed(`{"method":"item/completed","params":{"threadId":"thread-1","turnId":"t1","item":{"type":"commandExecution","id":"c1","command":"/bin/bash -lc 'ls'","status":"completed","exitCode":0,"aggregatedOutput":"x"}}}`)
	r.feed(`{"method":"turn/completed","params":{"threadId":"thread-1","turn":{"id":"t1","status":"completed","items":[]}}}`)
	if len(sink.background) != 0 {
		t.Errorf("background reported: %+v", sink.background)
	}
	if kinds := summary(sink.events); len(kinds) < 2 || !strings.HasPrefix(kinds[0], model.EventToolUse) {
		t.Errorf("events = %v", kinds)
	}
	for _, ev := range sink.events {
		if ev.AfterTurn {
			t.Errorf("an in-turn event is marked after its turn: %s", ev.Kind)
		}
	}
}

func TestShellCommandDropsTheLoginShellWrapper(t *testing.T) {
	for in, want := range map[string]string{
		`/bin/bash -lc 'sleep 30 && echo X'`: "sleep 30 && echo X",
		`/bin/bash -lc "npm test"`:           "npm test",
		`/bin/zsh -lc 'make'`:                "make",
		`python3 -m http.server`:             "python3 -m http.server",
	} {
		if got := shellCommand(in); got != want {
			t.Errorf("shellCommand(%q) = %q, want %q", in, got, want)
		}
	}
}
