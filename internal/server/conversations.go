package server

import (
	"net/http"
	"strconv"
	"time"

	"git.hq.quigley.com/research/roundhouse/internal/harness"
	"git.hq.quigley.com/research/roundhouse/internal/model"
	"git.hq.quigley.com/research/roundhouse/internal/service"
)

// conversationView is the wire form of a conversation: its recorded identity,
// its runtime as the service sees it now, and its current pending set, which is
// the only source of answerable requests.
type conversationView struct {
	ID             string
	ProjectID      string
	Name           string
	Label          string
	Harness        string
	NativeID       string
	Mode           string
	State          string
	TurnActive     bool
	Attention      string
	ReadSeq        int64
	LastSeq        int64
	OutputSeq      int64
	Model          string `dd:",+omitempty"`
	Version        string `dd:",+omitempty"`
	PermissionMode string `dd:",+omitempty"`
	Pending        []*requestView
}

type requestView struct {
	ID      string
	Kind    string
	Payload any `dd:",+opaque,+nullable"`
}

func newConversationView(conv service.ConversationView) *conversationView {
	view := &conversationView{
		ID:         conv.ID,
		ProjectID:  conv.ProjectID,
		Name:       conv.Name,
		Label:      conv.Label,
		Harness:    conv.Harness,
		NativeID:   conv.NativeID,
		Mode:       conv.Mode,
		State:      conv.State,
		TurnActive: conv.TurnActive,
		Attention:  conv.Attention,
		ReadSeq:    conv.ReadSeq,
		LastSeq:    conv.LastSeq,
		OutputSeq:  conv.OutputSeq,
		Pending:    make([]*requestView, 0, len(conv.Pending)),
	}
	if conv.Facts != nil {
		view.Model, view.Version, view.PermissionMode = conv.Facts.Model, conv.Facts.Version, conv.Facts.PermissionMode
	}
	for _, req := range conv.Pending {
		view.Pending = append(view.Pending, &requestView{ID: req.ID, Kind: req.Kind, Payload: req.Payload})
	}
	return view
}

type startConversationRequest struct {
	Harness string
	Name    string
	Prompt  string
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

type eventsView struct {
	Events []*model.Event
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
	conv, err := s.svc.StartConversation(r.PathValue("id"), req.Harness, req.Name, req.Prompt)
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

func (s *Server) getEvents(w http.ResponseWriter, r *http.Request) {
	var since int64
	if raw := r.URL.Query().Get("since"); raw != "" {
		parsed, err := strconv.ParseInt(raw, 10, 64)
		if err != nil || parsed < 0 {
			writeJSON(w, r, http.StatusBadRequest, &errorBody{Code: service.CodeInvalid, Message: "since must be a non-negative integer"})
			return
		}
		since = parsed
	}
	evs, err := s.svc.Events(r.PathValue("id"), since)
	if err != nil {
		writeError(w, r, err)
		return
	}
	if evs == nil {
		evs = []*model.Event{}
	}
	writeJSON(w, r, http.StatusOK, &eventsView{Events: evs})
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

func (s *Server) release(w http.ResponseWriter, r *http.Request) {
	s.command(w, r, s.svc.Release(r.PathValue("id")))
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
