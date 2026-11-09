package codex

import (
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/michaelquigley/df/dd"

	"git.hq.quigley.com/research/roundhouse/internal/model"
)

// maxText bounds the text an event carries; the full item stays in Raw.
const maxText = 4000

func bindItem(raw map[string]any) (*item, error) {
	it := &item{}
	if err := dd.Bind(it, raw); err != nil {
		return nil, fmt.Errorf("reading codex item: %w", err)
	}
	return it, nil
}

// mapItem turns one completed thread item into the events to record. the live
// stream and thread/read share it, so an item seen live and later imported
// yields events with the same NativeID. a user message the service sent carries
// the service's message identity as its clientId, which becomes its NativeID.
func mapItem(raw map[string]any) ([]*model.Event, error) {
	it, err := bindItem(raw)
	if err != nil {
		return nil, err
	}
	switch it.Type {
	case "userMessage":
		id := it.ID
		if it.ClientID != "" {
			id = it.ClientID
		}
		return []*model.Event{{Kind: model.EventUser, Text: clip(contentText(it.Content)), NativeID: id, Raw: raw}}, nil
	case "agentMessage":
		return []*model.Event{{Kind: model.EventAssistant, Text: clip(it.Text), NativeID: it.ID, Raw: raw}}, nil
	case "commandExecution":
		result := &model.Event{Kind: model.EventToolResult, ToolUseID: it.ID, Text: clip(it.AggregatedOutput), NativeID: it.ID + "/result"}
		if it.Status != "completed" || (it.ExitCode != nil && *it.ExitCode != 0) {
			result.Status = model.TurnError
			if result.Text == "" {
				result.Text = it.Status
			}
		}
		return []*model.Event{
			{Kind: model.EventToolUse, Tool: "shell", ToolUseID: it.ID, Text: clip("shell " + it.Command), NativeID: it.ID, Raw: raw},
			result,
		}, nil
	case "fileChange":
		var paths []string
		var diffs []string
		for _, change := range it.Changes {
			paths = append(paths, change.Path)
			diffs = append(diffs, change.Diff)
		}
		result := &model.Event{Kind: model.EventToolResult, ToolUseID: it.ID, Text: clip(strings.Join(diffs, "\n")), NativeID: it.ID + "/result"}
		if it.Status != "completed" {
			result.Status = model.TurnError
			result.Text = it.Status
		}
		return []*model.Event{
			{Kind: model.EventToolUse, Tool: "edit", ToolUseID: it.ID, Text: clip("edit " + strings.Join(paths, ", ")), NativeID: it.ID, Raw: raw},
			result,
		}, nil
	case "mcpToolCall":
		result := &model.Event{Kind: model.EventToolResult, ToolUseID: it.ID, Text: it.Status, NativeID: it.ID + "/result"}
		if it.Error != nil || it.Status == "failed" {
			result.Status = model.TurnError
		}
		tool := it.Server + "." + it.Tool
		return []*model.Event{
			{Kind: model.EventToolUse, Tool: tool, ToolUseID: it.ID, Text: clip(tool), NativeID: it.ID, Raw: raw},
			result,
		}, nil
	case "webSearch":
		return []*model.Event{{Kind: model.EventToolUse, Tool: "web search", ToolUseID: it.ID, Text: clip("web search " + it.Query), NativeID: it.ID, Raw: raw}}, nil
	default:
		return []*model.Event{{Kind: model.EventNative, Text: it.Type, NativeID: it.ID, Raw: raw}}, nil
	}
}

// questionPayload is what the operator sees for an agentMessage carrying
// questions, in the same plain shape as any question request.
func questionPayload(questions []asyncQuestion) map[string]any {
	var list []any
	for _, q := range questions {
		var options []any
		for _, label := range q.Options {
			options = append(options, map[string]any{"label": label})
		}
		list = append(list, map[string]any{"question": q.Title, "options": options})
	}
	return map[string]any{"tool": "question", "questions": list}
}

// answerText turns the operator's answers into the text codex reads back:
// the bare choice for one question, one line per question otherwise.
func answerText(answers map[string]string, questions []asyncQuestion) string {
	if len(questions) == 1 {
		if answer, ok := answers[questions[0].Title]; ok {
			return answer
		}
	}
	var lines []string
	for _, q := range questions {
		if answer, ok := answers[q.Title]; ok {
			lines = append(lines, q.Title+": "+answer)
		}
	}
	return strings.Join(lines, "\n")
}

func contentText(content any) string {
	list, _ := content.([]any)
	var parts []string
	for _, entry := range list {
		if block, ok := entry.(map[string]any); ok {
			if t, ok := block["text"].(string); ok {
				parts = append(parts, t)
			}
		}
	}
	return strings.Join(parts, "\n")
}

func turnStatus(status string) string {
	switch status {
	case "completed":
		return model.TurnSuccess
	case "interrupted":
		return model.TurnInterrupted
	default:
		return model.TurnError
	}
}

func clip(s string) string {
	if len(s) <= maxText {
		return s
	}
	cut := maxText
	for cut > 0 && !utf8.RuneStart(s[cut]) {
		cut--
	}
	return s[:cut] + "…"
}
