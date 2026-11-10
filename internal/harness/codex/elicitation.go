package codex

import (
	"fmt"
	"regexp"
	"slices"
	"sort"
	"strconv"

	"git.hq.quigley.com/research/roundhouse/internal/harness"
	"git.hq.quigley.com/research/roundhouse/internal/model"
)

// methodElicitation is how codex asks the client on behalf of an MCP server:
// for approval to run one of its tools, or for a form the server raised
// (capability report, codex-cli 0.166.0).
const methodElicitation = "mcpServer/elicitation/request"

// elicitationParams is the subset of McpServerElicitationRequestParams the
// adapter reads. _meta carries codex's own approval details.
type elicitationParams struct {
	TurnID          string         `dd:"turnId,+nullable"`
	ServerName      string         `dd:"serverName"`
	Mode            string         `dd:"mode"`
	Message         string         `dd:"message,+nullable"`
	RequestedSchema map[string]any `dd:"requestedSchema,+opaque,+nullable"`
	Meta            map[string]any `dd:"_meta,+opaque,+nullable"`
}

// elicitationResult is McpServerElicitationRequestResponse.
type elicitationResult struct {
	Action  string         `dd:"action"`
	Content map[string]any `dd:"content,+opaque,+omitempty"`
	Meta    map[string]any `dd:"_meta,+opaque,+omitempty"`
}

// elicitation is what an answer needs of a classified elicitation.
type elicitation struct {
	approval     bool        // an MCP tool approval
	allowSession bool        // codex offered "approve for this session"
	fields       []formField // a typed form's fields
}

// mcpCall is an MCP tool call in flight, from its item/started notice. the
// approval frame names the server but not the tool, so the tool comes from
// here.
type mcpCall struct {
	itemID string
	server string
	tool   string
}

// toolInMessage reads the tool name from codex's approval message, as a
// fallback when no call in flight names it.
var toolInMessage = regexp.MustCompile(`run tool "([^"]+)"`)

// classify sorts an elicitation into what roundhouse can put to the operator.
// an mcp_tool_call approval is a permission request; a typed form of primitive
// fields and single-select enums is a question; anything else (free-schema
// forms, url and user-verification modes, forms with no fields) cannot be
// answered, and ok is false.
func (r *runtime) classify(p *elicitationParams) (*elicitation, string, map[string]any, bool) {
	if kind, _ := p.Meta["codex_approval_kind"].(string); kind == "mcp_tool_call" {
		e := &elicitation{approval: true, allowSession: persistOffers(p.Meta, "session")}
		return e, model.RequestPermission, r.approvalPayloadFor(p, e), true
	}
	if p.Mode != "form" {
		return nil, "", nil, false
	}
	fields, ok := formFields(p.RequestedSchema)
	if !ok || len(fields) == 0 {
		return nil, "", nil, false
	}
	e := &elicitation{fields: fields}
	return e, model.RequestQuestion, formPayload(p, fields), true
}

func persistOffers(meta map[string]any, option string) bool {
	list, _ := meta["persist"].([]any)
	return slices.ContainsFunc(list, func(v any) bool { return v == option })
}

// approvalPayloadFor is the card for an MCP tool approval: the tool as
// server/tool, its description, its parameters as codex displays them, and
// whether codex offers approval for the rest of the session.
func (r *runtime) approvalPayloadFor(p *elicitationParams, e *elicitation) map[string]any {
	tool := r.toolInFlight(p.ServerName)
	if tool == "" {
		if m := toolInMessage.FindStringSubmatch(p.Message); m != nil {
			tool = m[1]
		}
	}
	payload := map[string]any{
		"tool":       p.ServerName + "/" + tool,
		"mcp_server": p.ServerName,
		"mcp_tool":   tool,
		"title":      p.Message,
	}
	if description, _ := p.Meta["tool_description"].(string); description != "" {
		payload["description"] = description
	}
	if params, ok := p.Meta["tool_params"].(map[string]any); ok {
		payload["input"] = params
	}
	if display, ok := p.Meta["tool_params_display"].([]any); ok {
		payload["parameters"] = display
	}
	if e.allowSession {
		payload["allow_session"] = true
	}
	return payload
}

// toolInFlight names the newest MCP call in flight on server, or "".
func (r *runtime) toolInFlight(server string) string {
	r.mu.Lock()
	defer r.mu.Unlock()
	for i := len(r.mcpCalls) - 1; i >= 0; i-- {
		if r.mcpCalls[i].server == server {
			return r.mcpCalls[i].tool
		}
	}
	return ""
}

// formField is one field of a typed form: string, number, integer, boolean or
// enum (a single choice among options).
type formField struct {
	Name        string
	Title       string
	Description string
	Type        string
	Options     []map[string]any // {value, label}, for an enum
	Required    bool
	Default     any
}

// formFields reads a typed form schema. it accepts only primitive fields and
// single-select enums; anything else, a multi-select or a nested object among
// them, makes the form unanswerable here. required fields come first, then the
// rest, each group by name.
func formFields(schema map[string]any) ([]formField, bool) {
	props, ok := schema["properties"].(map[string]any)
	if !ok {
		return nil, false
	}
	required := map[string]bool{}
	if list, ok := schema["required"].([]any); ok {
		for _, name := range list {
			if s, ok := name.(string); ok {
				required[s] = true
			}
		}
	}
	var fields []formField
	for name, raw := range props {
		prop, ok := raw.(map[string]any)
		if !ok {
			return nil, false
		}
		f := formField{Name: name, Required: required[name], Default: prop["default"]}
		f.Title, _ = prop["title"].(string)
		f.Description, _ = prop["description"].(string)
		switch t, _ := prop["type"].(string); t {
		case "string":
			f.Type = "string"
			if options := enumOptions(prop); options != nil {
				f.Type, f.Options = "enum", options
			}
		case "number", "integer", "boolean":
			f.Type = t
		default:
			return nil, false
		}
		fields = append(fields, f)
	}
	sort.Slice(fields, func(i, j int) bool {
		if fields[i].Required != fields[j].Required {
			return fields[i].Required
		}
		return fields[i].Name < fields[j].Name
	})
	return fields, true
}

// enumOptions reads a single-select enum: enum (with legacy enumNames as
// labels), or oneOf of {const, title}. it returns nil for a plain string.
func enumOptions(prop map[string]any) []map[string]any {
	var options []map[string]any
	if values, ok := prop["enum"].([]any); ok {
		names, _ := prop["enumNames"].([]any)
		for i, v := range values {
			label := fmt.Sprint(v)
			if i < len(names) {
				label = fmt.Sprint(names[i])
			}
			options = append(options, map[string]any{"value": fmt.Sprint(v), "label": label})
		}
		return options
	}
	if choices, ok := prop["oneOf"].([]any); ok {
		for _, c := range choices {
			choice, _ := c.(map[string]any)
			value := fmt.Sprint(choice["const"])
			label, _ := choice["title"].(string)
			if label == "" {
				label = value
			}
			options = append(options, map[string]any{"value": value, "label": label})
		}
		return options
	}
	return nil
}

// formPayload is the card for a typed form: what the server asked and its
// fields, plain data the UI renders generically.
func formPayload(p *elicitationParams, fields []formField) map[string]any {
	var list []any
	for _, f := range fields {
		field := map[string]any{"name": f.Name, "type": f.Type, "required": f.Required}
		if f.Title != "" {
			field["title"] = f.Title
		}
		if f.Description != "" {
			field["description"] = f.Description
		}
		if f.Options != nil {
			field["options"] = f.Options
		}
		if f.Default != nil {
			field["default"] = f.Default
		}
		list = append(list, field)
	}
	return map[string]any{"tool": "form", "mcp_server": p.ServerName, "message": p.Message, "fields": list}
}

// elicitationResponse answers an elicitation. an approval answers accept,
// accept for the session, or decline; codex's response carries no reason, so a
// denial's message stays in roundhouse's record. "always", which writes the
// operator's codex config, is never sent. a form answers accept with its
// fields as typed content; an empty optional field is left out.
func elicitationResponse(e *elicitation, answer harness.Answer) *elicitationResult {
	if e.approval {
		switch answer.Decision {
		case harness.DecisionAllow:
			return &elicitationResult{Action: "accept"}
		case harness.DecisionAllowSession:
			if e.allowSession {
				return &elicitationResult{Action: "accept", Meta: map[string]any{"persist": "session"}}
			}
			return &elicitationResult{Action: "accept"}
		default:
			return &elicitationResult{Action: "decline"}
		}
	}
	content := map[string]any{}
	for _, f := range e.fields {
		v, ok := answer.Answers[f.Name]
		if !ok || (v == "" && !f.Required) {
			continue
		}
		content[f.Name] = typedValue(f.Type, v)
	}
	return &elicitationResult{Action: "accept", Content: content}
}

// typedValue converts a field's answer to its schema type; a value that does
// not parse is sent as given, for the server to judge.
func typedValue(kind, v string) any {
	switch kind {
	case "number":
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			return f
		}
	case "integer":
		if i, err := strconv.ParseInt(v, 10, 64); err == nil {
			return i
		}
	case "boolean":
		if b, err := strconv.ParseBool(v); err == nil {
			return b
		}
	}
	return v
}

// unanswerableElicitation says what an elicitation roundhouse cannot put to
// the operator asked, for the record.
func unanswerableElicitation(p *elicitationParams) string {
	what := p.Message
	if what == "" {
		what = "(no message)"
	}
	return fmt.Sprintf("codex asked '%s' for MCP server '%s' in mode '%s', which roundhouse cannot answer: %q; interrupt the turn to go on", methodElicitation, p.ServerName, p.Mode, what)
}
