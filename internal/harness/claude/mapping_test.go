package claude

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
	evs, err := mapMessage(raw)
	if err != nil {
		t.Fatal(err)
	}
	return evs
}

func TestMessageTextAndToolResultsAreWhole(t *testing.T) {
	if len(long) < 3*4000 {
		t.Fatalf("fixture is only %d bytes", len(long))
	}
	cases := map[string]map[string]any{
		"prompt": {"type": "user", "uuid": "u-1", "message": map[string]any{"content": long}},
		"reply": {"type": "assistant", "uuid": "a-1", "message": map[string]any{"content": []any{
			map[string]any{"type": "text", "text": long},
		}}},
		"tool result": {"type": "user", "uuid": "r-1", "message": map[string]any{"content": []any{
			map[string]any{"type": "tool_result", "tool_use_id": "tu-1", "content": long},
		}}},
	}
	for name, raw := range cases {
		evs := mapOne(t, raw)
		if len(evs) != 1 || evs[0].Text != long {
			t.Errorf("%s: got %d events, text of %d bytes, want %d", name, len(evs), len(evs[0].Text), len(long))
		}
	}
}

func TestToolUseSummaryStaysBounded(t *testing.T) {
	evs := mapOne(t, map[string]any{"type": "assistant", "uuid": "a-2", "message": map[string]any{"content": []any{
		map[string]any{"type": "tool_use", "id": "tu-2", "name": "Bash", "input": map[string]any{"command": long}},
	}}})
	text := evs[0].Text
	if evs[0].Kind != model.EventToolUse || !strings.HasPrefix(text, "Bash line of agent output") || !strings.HasSuffix(text, "…") {
		t.Fatalf("summary = %q", text)
	}
	if len(text) > maxTitle+len("…") || !utf8.ValidString(text) {
		t.Errorf("summary is %d bytes (valid utf-8: %v), bound %d", len(text), utf8.ValidString(text), maxTitle)
	}
}
