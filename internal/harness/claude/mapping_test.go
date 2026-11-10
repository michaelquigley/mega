package claude

import (
	"encoding/json"
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

// the shape getSessionMessages synthesizes for a subagent's hand-back (claude
// code 2.1.318, sdk 0.3.318; capability report, 2026-11-10). the live stream
// never delivers it.
const handBack = `{"type":"user","uuid":"meta-1","session_id":"s-1","parent_tool_use_id":null,"is_meta":true,"isQueuedCommand":true,"message":{"role":"user","content":"<agent-message from=\"a1b2\">\nReport on the fetched page.\n</agent-message>"},"origin":{"kind":"peer","handback":true,"name":"web-fetch","from":"a1b2","senderTaskId":"t-1","body":"Report on the fetched page."}}`

func decode(t *testing.T, frame string) map[string]any {
	t.Helper()
	raw := map[string]any{}
	if err := json.Unmarshal([]byte(frame), &raw); err != nil {
		t.Fatal(err)
	}
	return raw
}

func TestMetaUserMessageIsNativeNotAPrompt(t *testing.T) {
	evs := mapOne(t, decode(t, handBack))
	if len(evs) != 1 || evs[0].Kind != model.EventNative || evs[0].Text != "meta: hand-back from web-fetch" || evs[0].NativeID != "meta-1" || evs[0].Raw == nil {
		t.Fatalf("hand-back mapped to %+v", evs)
	}
	// the transcript's own spelling, and no origin at all.
	plain := decode(t, `{"type":"user","uuid":"meta-2","isMeta":true,"message":{"content":"something claude added"}}`)
	if evs := mapOne(t, plain); evs[0].Kind != model.EventNative || evs[0].Text != "user/meta" {
		t.Errorf("meta without origin mapped to %+v", evs[0])
	}
}

// a prompt typed ahead while a turn ran is queued but is the operator's own.
func TestQueuedCommandWithoutMetaIsStillAPrompt(t *testing.T) {
	evs := mapOne(t, decode(t, `{"type":"user","uuid":"q-1","isQueuedCommand":true,"message":{"content":"and then run the tests"}}`))
	if len(evs) != 1 || evs[0].Kind != model.EventUser || evs[0].Text != "and then run the tests" {
		t.Errorf("queued prompt mapped to %+v", evs)
	}
}
