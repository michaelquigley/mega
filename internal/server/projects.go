package server

import (
	"net/http"
	"time"

	"github.com/michaelquigley/mega/internal/model"
	"github.com/michaelquigley/mega/internal/service"
)

// projectView is the wire form of a project. the trust notice is computed and
// not persisted, so it travels here rather than through the model's bindings.
type projectView struct {
	ID        string
	Name      string
	Checkout  string
	Starred   bool
	Untrusted bool
	// InheritedPermissionMode is the claude mode the operator's settings name
	// for the checkout, which a launch with no choice inherits; empty if none.
	InheritedPermissionMode string
	// CheckoutMissing says why the checkout cannot be worked in ("does not
	// exist", "not a directory", "not readable"); absent when it can.
	CheckoutMissing string `dd:",+omitempty"`
}

func newProjectView(project model.Project) *projectView {
	return &projectView{
		ID:                      project.ID,
		Name:                    project.Name,
		Checkout:                project.Checkout,
		Starred:                 project.Starred,
		Untrusted:               project.Untrusted,
		InheritedPermissionMode: project.InheritedPermissionMode,
		CheckoutMissing:         project.CheckoutMissing,
	}
}

type workspaceView struct {
	Projects      []*projectView
	Conversations []*conversationView
	NeedsYou      []string
	Shutdown      *shutdownView
}

// shutdownView is the "shut down when idle" gate: whether it is armed, since
// when, whether nothing is in the way, and what is, by conversation and kind.
type shutdownView struct {
	Armed    bool
	ArmedAt  time.Time `dd:",+omitempty"`
	Idle     bool
	Blockers []*blockerView
}

type blockerView struct {
	ConversationID string
	ProjectID      string
	Name           string
	Turn           bool
	Requests       int
	Background     int
	Settling       string `dd:",+omitempty"`
}

type createProjectRequest struct {
	Name     string
	Checkout string
}

// updateProjectRequest changes a project's name, its star, or both; a field
// left out is left as it is.
type updateProjectRequest struct {
	Name    *string `dd:",+omitempty"`
	Starred *bool   `dd:",+omitempty"`
}

func (s *Server) getWorkspace(w http.ResponseWriter, r *http.Request) {
	workspace := s.svc.Workspace()
	view := &workspaceView{
		Projects:      make([]*projectView, 0, len(workspace.Projects)),
		Conversations: make([]*conversationView, 0, len(workspace.Conversations)),
		NeedsYou:      workspace.NeedsYou,
		Shutdown: &shutdownView{
			Armed:    workspace.Gate.Armed,
			ArmedAt:  workspace.Gate.ArmedAt,
			Idle:     workspace.Gate.Idle(),
			Blockers: make([]*blockerView, 0, len(workspace.Gate.Blockers)),
		},
	}
	for _, b := range workspace.Gate.Blockers {
		view.Shutdown.Blockers = append(view.Shutdown.Blockers, &blockerView{ConversationID: b.ConversationID, ProjectID: b.ProjectID, Name: b.Name, Turn: b.Turn, Requests: b.Requests, Background: b.Background, Settling: b.Settling})
	}
	for _, project := range workspace.Projects {
		view.Projects = append(view.Projects, newProjectView(project))
	}
	for _, conv := range workspace.Conversations {
		view.Conversations = append(view.Conversations, newConversationView(conv))
	}
	writeJSON(w, r, http.StatusOK, view)
}

func (s *Server) createProject(w http.ResponseWriter, r *http.Request) {
	var req createProjectRequest
	if !readJSON(w, r, &req) {
		return
	}
	project, err := s.svc.RegisterProject(req.Name, req.Checkout)
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeJSON(w, r, http.StatusCreated, newProjectView(project))
}

func (s *Server) updateProject(w http.ResponseWriter, r *http.Request) {
	var req updateProjectRequest
	if !readJSON(w, r, &req) {
		return
	}
	project, err := s.svc.UpdateProject(r.PathValue("id"), service.ProjectChange{Name: req.Name, Starred: req.Starred})
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeJSON(w, r, http.StatusOK, newProjectView(project))
}

func (s *Server) deleteProject(w http.ResponseWriter, r *http.Request) {
	if err := s.svc.RemoveProject(r.PathValue("id")); err != nil {
		writeError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

type pathsView struct {
	Paths []string
}

// paths searches a project's checkout by name. directories end in a slash.
func (s *Server) paths(w http.ResponseWriter, r *http.Request) {
	paths, err := s.svc.Paths(r.PathValue("id"), r.URL.Query().Get("q"))
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeJSON(w, r, http.StatusOK, &pathsView{Paths: paths})
}
