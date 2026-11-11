package claude

import (
	"fmt"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/michaelquigley/df/dd"

	"github.com/michaelquigley/mega/internal/model"
)

// maxTitle bounds a tool-use summary line, in bytes.
const maxTitle = 200

// sdkMessage is the subset of an agent sdk message (and of a stored session
// message, which shares the user and assistant shapes) the adapter reads.
type sdkMessage struct {
	Type              string
	Subtype           string
	UUID              string
	SessionID         string
	ParentToolUseID   string `dd:",+nullable"`
	Message           *sdkContent
	Model             string
	ClaudeCodeVersion string
	PermissionMode    string `dd:"permissionMode"`
	Cwd               string
	IsError           bool
	Event             map[string]any `dd:",+opaque,+nullable"`

	// IsMeta marks a user message claude added itself, such as a subagent's
	// hand-back, which the history reader synthesizes and the live stream
	// never delivers. the reader spells it is_meta; isMeta is the transcript's
	// own spelling. IsQueuedCommand alone is a prompt typed ahead.
	IsMeta          bool           `dd:"is_meta,+nullable"`
	IsMetaCamel     bool           `dd:"isMeta,+nullable"`
	IsQueuedCommand bool           `dd:"isQueuedCommand,+nullable"`
	Origin          map[string]any `dd:",+opaque,+nullable"`
}

type sdkContent struct {
	Content any `dd:",+opaque,+nullable"`
}

type sdkBlock struct {
	Type      string
	Text      string
	ID        string
	Name      string
	Input     any `dd:",+opaque,+nullable"`
	ToolUseID string
	Content   any `dd:",+opaque,+nullable"`
	IsError   bool
}

type streamEvent struct {
	Type  string
	Delta struct {
		Type string
		Text string
	}
}

func bindMessage(raw map[string]any) (*sdkMessage, error) {
	msg := &sdkMessage{}
	if err := dd.Bind(msg, raw); err != nil {
		return nil, fmt.Errorf("reading sdk message: %w", err)
	}
	return msg, nil
}

// mapMessage turns one sdk message into the events to record. live stream and
// stored history share it, so a message seen live and later imported yields
// events with the same NativeID.
func mapMessage(raw map[string]any) ([]*model.Event, error) {
	msg, err := bindMessage(raw)
	if err != nil {
		return nil, err
	}
	switch {
	case msg.Type == "assistant" && msg.ParentToolUseID == "":
		return mapBlocks(msg, raw, assistantBlock)
	case msg.Type == "user" && msg.ParentToolUseID == "" && (msg.IsMeta || msg.IsMetaCamel):
		// not the operator's: claude's own meta message, such as a subagent's
		// hand-back. kept under the native toggle with its identity, so an
		// import stays idempotent.
		return []*model.Event{{Kind: model.EventNative, Text: metaText(msg.Origin), NativeID: msg.UUID, Raw: raw}}, nil
	case msg.Type == "user" && msg.ParentToolUseID == "":
		if text, ok := msg.Message.text(); ok {
			return []*model.Event{{Kind: model.EventUser, Text: text, NativeID: msg.UUID, Raw: raw}}, nil
		}
		return mapBlocks(msg, raw, userBlock)
	case msg.Type == "result":
		status := model.TurnSuccess
		if msg.IsError || msg.Subtype != "success" {
			status = model.TurnError
		}
		return []*model.Event{{Kind: model.EventTurnEnd, Status: status, Text: msg.Subtype, NativeID: msg.UUID, Raw: raw}}, nil
	default:
		return []*model.Event{nativeEvent(msg, raw)}, nil
	}
}

// metaText names a meta user message from its origin: "meta: hand-back from
// web-fetch" for a subagent's hand-back, "meta: <kind> from <name>" or
// "meta: <kind>" otherwise, and "user/meta" when there is no origin.
func metaText(origin map[string]any) string {
	kind, _ := origin["kind"].(string)
	if kind == "" {
		return "user/meta"
	}
	if handback, _ := origin["handback"].(bool); handback {
		kind = "hand-back"
	}
	if name, _ := origin["name"].(string); name != "" {
		return fmt.Sprintf("meta: %s from %s", kind, name)
	}
	return "meta: " + kind
}

func nativeEvent(msg *sdkMessage, raw map[string]any) *model.Event {
	text := msg.Type
	if msg.Subtype != "" {
		text += "/" + msg.Subtype
	}
	return &model.Event{Kind: model.EventNative, Text: text, NativeID: msg.UUID, Raw: raw}
}

// mapBlocks makes one event per content block. the sdk streams one block per
// message, so NativeID is the message uuid; any further block gets a suffix.
func mapBlocks(msg *sdkMessage, raw map[string]any, mapBlock func(*sdkBlock) *model.Event) ([]*model.Event, error) {
	blocks, err := msg.Message.blocks()
	if err != nil {
		return nil, err
	}
	var evs []*model.Event
	for i, block := range blocks {
		ev := mapBlock(block)
		ev.NativeID = msg.UUID
		if i > 0 {
			ev.NativeID = fmt.Sprintf("%s/%d", msg.UUID, i)
		}
		ev.Raw = raw
		evs = append(evs, ev)
	}
	if len(evs) == 0 {
		evs = append(evs, nativeEvent(msg, raw))
	}
	return evs, nil
}

func assistantBlock(block *sdkBlock) *model.Event {
	switch block.Type {
	case "text":
		return &model.Event{Kind: model.EventAssistant, Text: block.Text}
	case "tool_use":
		return &model.Event{Kind: model.EventToolUse, Tool: block.Name, ToolUseID: block.ID, Text: title(strings.TrimSpace(block.Name + " " + summarize(block.Input)))}
	default:
		return &model.Event{Kind: model.EventNative, Text: "assistant/" + block.Type}
	}
}

func userBlock(block *sdkBlock) *model.Event {
	if block.Type != "tool_result" {
		return &model.Event{Kind: model.EventNative, Text: "user/" + block.Type + " " + block.Text}
	}
	ev := &model.Event{Kind: model.EventToolResult, ToolUseID: block.ToolUseID, Text: resultText(block.Content)}
	if block.IsError {
		ev.Status = model.TurnError
	}
	return ev
}

// text returns the content when it is a plain string, which is how a typed
// prompt is stored.
func (c *sdkContent) text() (string, bool) {
	if c == nil {
		return "", false
	}
	s, ok := c.Content.(string)
	return s, ok
}

func (c *sdkContent) blocks() ([]*sdkBlock, error) {
	if c == nil {
		return nil, nil
	}
	list, _ := c.Content.([]any)
	var blocks []*sdkBlock
	for _, item := range list {
		data, ok := item.(map[string]any)
		if !ok {
			continue
		}
		block := &sdkBlock{}
		if err := dd.Bind(block, data); err != nil {
			return nil, fmt.Errorf("reading content block: %w", err)
		}
		blocks = append(blocks, block)
	}
	return blocks, nil
}

// delta returns the streamed text in a partial message, if it carries any.
func delta(msg *sdkMessage) string {
	if msg.Type != "stream_event" || msg.ParentToolUseID != "" || msg.Event == nil {
		return ""
	}
	ev := &streamEvent{}
	if err := dd.Bind(ev, msg.Event); err != nil {
		return ""
	}
	if ev.Type != "content_block_delta" || ev.Delta.Type != "text_delta" {
		return ""
	}
	return ev.Delta.Text
}

func resultText(content any) string {
	switch v := content.(type) {
	case string:
		return v
	case []any:
		var parts []string
		for _, item := range v {
			if block, ok := item.(map[string]any); ok {
				if text, ok := block["text"].(string); ok {
					parts = append(parts, text)
				}
			}
		}
		return strings.Join(parts, "\n")
	default:
		return ""
	}
}

// summaryKeys are the tool input fields that say most about a call, in order.
var summaryKeys = []string{"command", "file_path", "path", "pattern", "url", "query", "description"}

// summarize names a tool call by its most telling input, falling back to the
// sorted scalar fields. the full input stays in Raw.
func summarize(input any) string {
	fields, ok := input.(map[string]any)
	if !ok {
		return ""
	}
	for _, key := range summaryKeys {
		if value, ok := fields[key].(string); ok && value != "" {
			return value
		}
	}
	keys := make([]string, 0, len(fields))
	for key := range fields {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	var parts []string
	for _, key := range keys {
		switch value := fields[key].(type) {
		case string, float64, bool:
			parts = append(parts, fmt.Sprintf("%s=%v", key, value))
		}
	}
	return strings.Join(parts, " ")
}

// title bounds a tool-use summary line, which the view shows as one line and
// which is a title by design; the full call stays in Raw. message text and
// tool results are never bounded.
func title(s string) string {
	if len(s) <= maxTitle {
		return s
	}
	cut := maxTitle
	for cut > 0 && !utf8.RuneStart(s[cut]) {
		cut--
	}
	return s[:cut] + "…"
}
