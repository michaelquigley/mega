package server

import (
	"net/http"
	"strconv"
	"time"

	"github.com/michaelquigley/mega/internal/harness"
	"github.com/michaelquigley/mega/internal/model"
	"github.com/michaelquigley/mega/internal/service"
)

// conversationView is the wire form of a conversation: its recorded identity,
// its runtime as the service sees it now, and its current pending set, which is
// the only source of answerable requests.
type conversationView struct {
	ID         string
	ProjectID  string
	Name       string
	Label      string
	Harness    string
	NativeID   string
	Mode       string
	State      string
	TurnActive bool
	// TurnStartedAt is when the turn in flight began; absent when none is.
	TurnStartedAt  time.Time `dd:",+omitempty"`
	Attention      string
	ReadSeq        int64
	LastSeq        int64
	OutputSeq      int64
	Model          string `dd:",+omitempty"`
	Version        string `dd:",+omitempty"`
	PermissionMode string `dd:",+omitempty"`
	// PermissionModeInherited is whether the running mode came from the
	// operator's settings because mega passed none.
	PermissionModeInherited bool
	// PermissionModeRequested is whether the running mode is only the one the
	// runtime was launched with, not yet reported by the harness.
	PermissionModeRequested bool `dd:",+omitempty"`
	// PermissionModeChoice is the recorded choice every launch passes; empty
	// is mega's default.
	PermissionModeChoice string
	// ModelChoice and EffortChoice are the recorded choice every launch (and,
	// for codex, every prompt) passes; empty inherits.
	ModelChoice  string `dd:",+omitempty"`
	EffortChoice string `dd:",+omitempty"`
	// Effort is the effort the harness reported, where it reports one (codex).
	Effort string `dd:",+omitempty"`
	// ModelNext and EffortNext are a change asked for while running that the
	// next prompt carries.
	ModelNext  string `dd:",+omitempty"`
	EffortNext string `dd:",+omitempty"`
	// ModelNextInherit and EffortNextInherit say the next prompt returns that
	// field to inherit (codex only).
	ModelNextInherit  bool `dd:",+omitempty"`
	EffortNextInherit bool `dd:",+omitempty"`
	// Background is the work the runtime keeps running outside a turn;
	// absent when none.
	Background []*backgroundView `dd:",+omitempty"`
	// MCP counts the runtime's MCP servers by status; absent until it has
	// reported them, and with no runtime. the servers themselves are on
	// their own read.
	MCP *mcpSummaryView `dd:",+omitempty"`
	// Environment is what the running runtime was launched with, as names and
	// fingerprints; absent when no runtime runs. EnvironmentStale says it
	// would start in another environment now.
	Environment      *runtimeEnvironmentView `dd:",+omitempty"`
	EnvironmentStale bool                    `dd:",+omitempty"`
	Pending          []*requestView
}

// backgroundView is one background job: the harness's id for it, what it is,
// its kind, and when the adapter first saw it.
type backgroundView struct {
	ID          string
	Description string
	Kind        string
	StartedAt   time.Time `dd:",+omitempty"`
}

type requestView struct {
	ID      string
	Kind    string
	Payload any `dd:",+opaque,+nullable"`
	// At is when the request was recorded: its request event's own time.
	At time.Time `dd:",+omitempty"`
}

func newConversationView(conv service.ConversationView) *conversationView {
	view := &conversationView{
		ID:            conv.ID,
		ProjectID:     conv.ProjectID,
		Name:          conv.Name,
		Label:         conv.Label,
		Harness:       conv.Harness,
		NativeID:      conv.NativeID,
		Mode:          conv.Mode,
		State:         conv.State,
		TurnActive:    conv.TurnActive,
		TurnStartedAt: conv.TurnStartedAt,
		Attention:     conv.Attention,
		ReadSeq:       conv.ReadSeq,
		LastSeq:       conv.LastSeq,
		OutputSeq:     conv.OutputSeq,

		PermissionModeChoice: conv.PermissionMode,
		ModelChoice:          conv.Model,
		EffortChoice:         conv.Effort,
		ModelNext:            conv.ModelNext,
		EffortNext:           conv.EffortNext,
		ModelNextInherit:     conv.ModelNextInherit,
		EffortNextInherit:    conv.EffortNextInherit,
		Pending:              make([]*requestView, 0, len(conv.Pending)),
	}
	if conv.Facts != nil {
		view.Model, view.Version, view.PermissionMode = conv.Facts.Model, conv.Facts.Version, conv.Facts.PermissionMode
		view.PermissionModeInherited = conv.Facts.ModeInherited
		view.PermissionModeRequested = conv.Facts.ModeRequested
		view.Effort = conv.Facts.Effort
	}
	if e := conv.Environment; e != nil {
		view.Environment = &runtimeEnvironmentView{Source: e.Source, Activation: e.Activation, Changed: append([]string{}, e.Changed...), Fingerprint: e.Fingerprint}
		view.EnvironmentStale = conv.EnvironmentStale
	}
	for _, job := range conv.Background {
		view.Background = append(view.Background, &backgroundView{ID: job.ID, Description: job.Description, Kind: job.Kind, StartedAt: job.StartedAt})
	}
	for _, req := range conv.Pending {
		view.Pending = append(view.Pending, &requestView{ID: req.ID, Kind: req.Kind, Payload: req.Payload, At: conv.PendingAt[req.ID]})
	}
	view.MCP = newMCPSummary(conv)
	return view
}

type startConversationRequest struct {
	Harness        string
	Name           string
	Prompt         string
	PermissionMode string
	Model          string
	Effort         string
}

type permissionModeRequest struct {
	Mode string
}

type modelRequest struct {
	Model  string
	Effort string
}

// modelOptionView is one model a harness offers.
type modelOptionView struct {
	ID            string
	Resolved      string `dd:",+omitempty"`
	DisplayName   string `dd:",+omitempty"`
	Description   string `dd:",+omitempty"`
	Efforts       []string
	DefaultEffort string `dd:",+omitempty"`
	IsDefault     bool
}

// inheritedView is what a launch with no choice gets in the checkout, where
// the harness says.
type inheritedView struct {
	Model  string `dd:",+omitempty"`
	Effort string `dd:",+omitempty"`
}

type modelsView struct {
	Version   string `dd:",+omitempty"`
	Models    []*modelOptionView
	Inherited *inheritedView
}

// modeOutcomeView is the harness's answer to a switch: confirmed, refused or
// unconfirmed, with what it said.
type modeOutcomeView struct {
	Outcome string
	Message string `dd:",+omitempty"`
}

type updateConversationRequest struct {
	Name  string
	Label string
}

type promptRequest struct {
	Text string
}

type readRequest struct {
	Seq int64
}

type answerRequest struct {
	RequestID string
	Decision  string
	Message   string
	Answers   map[string]string
}

// eventsView is a stretch of history, From through Through. native events are
// left out unless asked for, but the stretch covers them. Earlier is present
// for a read by turns: what lies before From.
type eventsView struct {
	Events  []*model.Event
	From    int64
	Through int64
	Earlier *earlierView `dd:",+omitempty"`
}

type earlierView struct {
	Turns  int
	Events int64
}

type sessionView struct {
	NativeID  string
	Preview   string
	CreatedAt time.Time `dd:",+omitempty"`
	UpdatedAt time.Time `dd:",+omitempty"`
}

type sessionsView struct {
	Sessions []*sessionView
}

type adoptRequest struct {
	Harness  string
	NativeID string
	Name     string
}

func (s *Server) discover(w http.ResponseWriter, r *http.Request) {
	sessions, err := s.svc.Discover(r.PathValue("id"), r.URL.Query().Get("harness"))
	if err != nil {
		writeError(w, r, err)
		return
	}
	view := &sessionsView{Sessions: make([]*sessionView, 0, len(sessions))}
	for _, session := range sessions {
		view.Sessions = append(view.Sessions, &sessionView{NativeID: session.NativeID, Preview: session.Preview, CreatedAt: session.CreatedAt, UpdatedAt: session.UpdatedAt})
	}
	writeJSON(w, r, http.StatusOK, view)
}

func (s *Server) adopt(w http.ResponseWriter, r *http.Request) {
	var req adoptRequest
	if !readJSON(w, r, &req) {
		return
	}
	conv, err := s.svc.Adopt(r.PathValue("id"), req.Harness, req.NativeID, req.Name)
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeJSON(w, r, http.StatusCreated, newConversationView(conv))
}

func (s *Server) startConversation(w http.ResponseWriter, r *http.Request) {
	var req startConversationRequest
	if !readJSON(w, r, &req) {
		return
	}
	conv, err := s.svc.StartConversation(r.PathValue("id"), req.Harness, req.Name, req.Prompt, service.LaunchChoice{PermissionMode: req.PermissionMode, Model: req.Model, Effort: req.Effort})
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeJSON(w, r, http.StatusCreated, newConversationView(conv))
}

func (s *Server) updateConversation(w http.ResponseWriter, r *http.Request) {
	var req updateConversationRequest
	if !readJSON(w, r, &req) {
		return
	}
	conv, err := s.svc.UpdateConversation(r.PathValue("id"), req.Name, req.Label)
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeJSON(w, r, http.StatusOK, newConversationView(conv))
}

func (s *Server) deleteConversation(w http.ResponseWriter, r *http.Request) {
	if err := s.svc.RemoveConversation(r.PathValue("id")); err != nil {
		writeError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// getEvents reads history: ?turns=N for the newest N turns and the turn in
// flight, ?before=S&turns=N for the N turns before seq S (all of them without
// turns), or ?since=N for everything after seq N. native events come only
// with native=1.
func (s *Server) getEvents(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()
	var q service.HistoryQuery
	for _, field := range []struct {
		name string
		into *int64
	}{{"since", &q.Since}, {"before", &q.Before}} {
		if raw := query.Get(field.name); raw != "" {
			parsed, err := strconv.ParseInt(raw, 10, 64)
			if err != nil || parsed < 0 {
				writeJSON(w, r, http.StatusBadRequest, &errorBody{Code: service.CodeInvalid, Message: field.name + " must be a non-negative integer"})
				return
			}
			*field.into = parsed
		}
	}
	if raw := query.Get("turns"); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil || parsed < 1 {
			writeJSON(w, r, http.StatusBadRequest, &errorBody{Code: service.CodeInvalid, Message: "turns must be a positive integer"})
			return
		}
		q.Turns = parsed
	}
	if query.Get("since") != "" && (q.Turns > 0 || q.Before > 0) {
		writeJSON(w, r, http.StatusBadRequest, &errorBody{Code: service.CodeInvalid, Message: "since reads everything after an event; it does not combine with turns or before"})
		return
	}
	switch raw := query.Get("native"); raw {
	case "", "0", "false":
	case "1", "true":
		q.Native = true
	default:
		writeJSON(w, r, http.StatusBadRequest, &errorBody{Code: service.CodeInvalid, Message: "native must be 1 or 0"})
		return
	}
	h, err := s.svc.History(r.PathValue("id"), q)
	if err != nil {
		writeError(w, r, err)
		return
	}
	view := &eventsView{Events: h.Events, From: h.From, Through: h.Through}
	if view.Events == nil {
		view.Events = []*model.Event{}
	}
	if h.Windowed {
		view.Earlier = &earlierView{Turns: h.EarlierTurns, Events: h.EarlierEvents()}
	}
	writeJSON(w, r, http.StatusOK, view)
}

func (s *Server) prompt(w http.ResponseWriter, r *http.Request) {
	var req promptRequest
	if !readJSON(w, r, &req) {
		return
	}
	s.command(w, r, s.svc.Prompt(r.PathValue("id"), req.Text))
}

func (s *Server) answer(w http.ResponseWriter, r *http.Request) {
	var req answerRequest
	if !readJSON(w, r, &req) {
		return
	}
	answer := harness.Answer{Decision: req.Decision, Message: req.Message, Answers: req.Answers}
	s.command(w, r, s.svc.Answer(r.PathValue("id"), req.RequestID, answer))
}

func (s *Server) read(w http.ResponseWriter, r *http.Request) {
	var req readRequest
	if !readJSON(w, r, &req) {
		return
	}
	s.command(w, r, s.svc.Read(r.PathValue("id"), req.Seq))
}

func (s *Server) interrupt(w http.ResponseWriter, r *http.Request) {
	s.command(w, r, s.svc.Interrupt(r.PathValue("id")))
}

func (s *Server) setPermissionMode(w http.ResponseWriter, r *http.Request) {
	var req permissionModeRequest
	if !readJSON(w, r, &req) {
		return
	}
	outcome, err := s.svc.SetPermissionMode(r.PathValue("id"), req.Mode)
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeJSON(w, r, http.StatusOK, &modeOutcomeView{Outcome: outcome.Outcome, Message: outcome.Message})
}

func (s *Server) models(w http.ResponseWriter, r *http.Request) {
	list, err := s.svc.Models(r.PathValue("id"), r.URL.Query().Get("harness"))
	if err != nil {
		writeError(w, r, err)
		return
	}
	view := &modelsView{
		Version:   list.Version,
		Models:    make([]*modelOptionView, 0, len(list.Models)),
		Inherited: &inheritedView{Model: list.InheritedModel, Effort: list.InheritedEffort},
	}
	for _, m := range list.Models {
		efforts := m.Efforts
		if efforts == nil {
			efforts = []string{}
		}
		view.Models = append(view.Models, &modelOptionView{ID: m.ID, Resolved: m.Resolved, DisplayName: m.DisplayName, Description: m.Description, Efforts: efforts, DefaultEffort: m.DefaultEffort, IsDefault: m.IsDefault})
	}
	writeJSON(w, r, http.StatusOK, view)
}

func (s *Server) setModel(w http.ResponseWriter, r *http.Request) {
	var req modelRequest
	if !readJSON(w, r, &req) {
		return
	}
	outcome, err := s.svc.SetModel(r.PathValue("id"), req.Model, req.Effort)
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeJSON(w, r, http.StatusOK, &modeOutcomeView{Outcome: outcome.Outcome, Message: outcome.Message})
}

func (s *Server) release(w http.ResponseWriter, r *http.Request) {
	s.command(w, r, s.svc.Release(r.PathValue("id")))
}

// start launches a stopped conversation's runtime with no prompt.
func (s *Server) start(w http.ResponseWriter, r *http.Request) {
	s.command(w, r, s.svc.Start(r.PathValue("id")))
}

func (s *Server) resume(w http.ResponseWriter, r *http.Request) {
	s.command(w, r, s.svc.Resume(r.PathValue("id")))
}

// command answers an accepted command with 202: its effects arrive on the
// stream as confirmed events and workspace changes.
func (s *Server) command(w http.ResponseWriter, r *http.Request, err error) {
	if err != nil {
		writeError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusAccepted)
}

// runtimeEnvironmentView is a runtime's launch environment: where its
// activation came from and that activation's fingerprint, the names it
// changed, and the fingerprint of the whole. never a value.
type runtimeEnvironmentView struct {
	Source      string
	Activation  string `dd:",+omitempty"`
	Changed     []string
	Fingerprint string
}
