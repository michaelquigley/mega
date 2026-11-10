package codex

import (
	"strings"
	"testing"
	"unicode/utf8"

	"git.hq.quigley.com/research/roundhouse/internal/model"
)

// long is well past the old 4000-byte bound, with multibyte runes throughout,
// so a cut anywhere would show.
var long = strings.Repeat("line of agent output, naïve café ✓\n", 400)

func mapOne(t *testing.T, raw map[string]any) []*model.Event {
	t.Helper()
	evs, err := mapItem(raw)
	if err != nil {
		t.Fatal(err)
	}
	return evs
}

func TestMessageTextAndToolResultsAreWhole(t *testing.T) {
	if len(long) < 3*4000 {
		t.Fatalf("fixture is only %d bytes", len(long))
	}
	checks := []struct {
		name string
		raw  map[string]any
		kind string
	}{
		{"prompt", map[string]any{"type": "userMessage", "id": "u-1", "content": []any{map[string]any{"type": "text", "text": long}}}, model.EventUser},
		{"reply", map[string]any{"type": "agentMessage", "id": "a-1", "text": long}, model.EventAssistant},
		{"command output", map[string]any{"type": "commandExecution", "id": "c-1", "command": "cat big", "cwd": "/f", "commandActions": []any{}, "status": "completed", "exitCode": 0, "aggregatedOutput": long}, model.EventToolResult},
		{"diff", map[string]any{"type": "fileChange", "id": "f-1", "status": "completed", "changes": []any{map[string]any{"path": "a.go", "kind": map[string]any{"type": "update"}, "diff": long}}}, model.EventToolResult},
	}
	for _, c := range checks {
		var found *model.Event
		for _, ev := range mapOne(t, c.raw) {
			if ev.Kind == c.kind {
				found = ev
			}
		}
		if found == nil || found.Text != long {
			t.Errorf("%s: no whole %s event", c.name, c.kind)
		}
	}
}

func TestToolUseSummaryStaysBounded(t *testing.T) {
	evs := mapOne(t, map[string]any{"type": "commandExecution", "id": "c-2", "command": long, "cwd": "/f", "commandActions": []any{}, "status": "completed", "exitCode": 0, "aggregatedOutput": "ok"})
	text := evs[0].Text
	if evs[0].Kind != model.EventToolUse || !strings.HasPrefix(text, "shell line of agent output") || !strings.HasSuffix(text, "…") {
		t.Fatalf("summary = %q", text)
	}
	if len(text) > maxTitle+len("…") || !utf8.ValidString(text) {
		t.Errorf("summary is %d bytes (valid utf-8: %v), bound %d", len(text), utf8.ValidString(text), maxTitle)
	}
}
