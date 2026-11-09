// Package server exposes the service over HTTP and WebSocket and serves the
// embedded UI. it holds no workspace state.
package server

import (
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/gorilla/websocket"
	"github.com/michaelquigley/df/dd"
	"github.com/michaelquigley/df/dl"

	"git.hq.quigley.com/research/roundhouse/internal/service"
	"git.hq.quigley.com/research/roundhouse/ui"
)

const (
	maxRequestBody    = 1 << 20
	defaultHeartbeat  = 20 * time.Second
	streamWriteWait   = 10 * time.Second
	streamReadLimit   = 4096
	heartbeatsPerRead = 3 // a peer may miss this many heartbeats before it is presumed gone
)

// Server adapts the service to its HTTP surface.
type Server struct {
	svc       *service.Service
	heartbeat time.Duration
	upgrader  websocket.Upgrader
}

// New returns a server over svc.
func New(svc *service.Service) *Server {
	return &Server{svc: svc, heartbeat: defaultHeartbeat}
}

// Handler returns the complete HTTP surface: API and stream under /api/, the
// embedded UI behind everything else, and the loopback guard in front of both.
func (s *Server) Handler() http.Handler {
	return guard(ui.Middleware(s.api()))
}

func (s *Server) api() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/workspace", s.getWorkspace)
	mux.HandleFunc("POST /api/projects", s.createProject)
	mux.HandleFunc("PATCH /api/projects/{id}", s.updateProject)
	mux.HandleFunc("DELETE /api/projects/{id}", s.deleteProject)
	mux.HandleFunc("POST /api/projects/{id}/conversations", s.startConversation)
	mux.HandleFunc("GET /api/projects/{id}/discover", s.discover)
	mux.HandleFunc("POST /api/projects/{id}/adopt", s.adopt)
	mux.HandleFunc("PATCH /api/conversations/{id}", s.updateConversation)
	mux.HandleFunc("GET /api/conversations/{id}/events", s.getEvents)
	mux.HandleFunc("POST /api/conversations/{id}/prompt", s.prompt)
	mux.HandleFunc("POST /api/conversations/{id}/answer", s.answer)
	mux.HandleFunc("POST /api/conversations/{id}/interrupt", s.interrupt)
	mux.HandleFunc("POST /api/conversations/{id}/release", s.release)
	mux.HandleFunc("POST /api/conversations/{id}/resume", s.resume)
	mux.HandleFunc("GET /api/stream", s.stream)
	return mux
}

type errorBody struct {
	Code    string
	Message string
}

func writeJSON(w http.ResponseWriter, r *http.Request, status int, body any) {
	data, err := dd.UnbindJSON(body)
	if err != nil {
		dl.Errorf("encoding response for '%s %s': %v", r.Method, r.URL.Path, err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if _, err := w.Write(data); err != nil {
		dl.Debugf("writing response for '%s %s': %v", r.Method, r.URL.Path, err)
	}
}

// writeError maps a service error to its status. anything without a code is an
// internal fault: it is logged in full and reported without detail.
func writeError(w http.ResponseWriter, r *http.Request, err error) {
	code := service.ErrorCode(err)
	switch code {
	case service.CodeInvalid:
		writeJSON(w, r, http.StatusBadRequest, &errorBody{Code: code, Message: err.Error()})
	case service.CodeNotFound:
		writeJSON(w, r, http.StatusNotFound, &errorBody{Code: code, Message: err.Error()})
	case service.CodeConflict, service.CodeNotPending:
		writeJSON(w, r, http.StatusConflict, &errorBody{Code: code, Message: err.Error()})
	case service.CodeUnavailable:
		writeJSON(w, r, http.StatusServiceUnavailable, &errorBody{Code: code, Message: err.Error()})
	default:
		dl.Errorf("'%s %s': %v", r.Method, r.URL.Path, err)
		writeJSON(w, r, http.StatusInternalServerError, &errorBody{Code: "internal", Message: "internal error"})
	}
}

// readJSON binds a request body. dd reports a malformed body as an ordinary
// error, which is the caller's fault rather than an internal one.
func readJSON(w http.ResponseWriter, r *http.Request, target any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, maxRequestBody)
	if err := dd.BindJSONReader(target, r.Body); err != nil {
		status := http.StatusBadRequest
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			status = http.StatusRequestEntityTooLarge
		}
		writeJSON(w, r, status, &errorBody{Code: service.CodeInvalid, Message: fmt.Sprintf("reading request body: %v", err)})
		return false
	}
	return true
}
