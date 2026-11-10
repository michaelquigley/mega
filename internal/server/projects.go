package server

import (
	"net/http"

	"git.hq.quigley.com/research/roundhouse/internal/model"
)

// projectView is the wire form of a project. the trust notice is computed and
// not persisted, so it travels here rather than through the model's bindings.
type projectView struct {
	ID        string
	Name      string
	Checkout  string
	Untrusted bool
	// InheritedPermissionMode is the claude mode the operator's settings name
	// for the checkout, which a launch with no choice inherits; empty if none.
	InheritedPermissionMode string
}

func newProjectView(project model.Project) *projectView {
	return &projectView{
		ID:                      project.ID,
		Name:                    project.Name,
		Checkout:                project.Checkout,
		Untrusted:               project.Untrusted,
		InheritedPermissionMode: project.InheritedPermissionMode,
	}
}

type workspaceView struct {
	Projects      []*projectView
	Conversations []*conversationView
	NeedsYou      []string
}

type createProjectRequest struct {
	Name     string
	Checkout string
}

type updateProjectRequest struct {
	Name string
}

func (s *Server) getWorkspace(w http.ResponseWriter, r *http.Request) {
	workspace := s.svc.Workspace()
	view := &workspaceView{
		Projects:      make([]*projectView, 0, len(workspace.Projects)),
		Conversations: make([]*conversationView, 0, len(workspace.Conversations)),
		NeedsYou:      workspace.NeedsYou,
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
	project, err := s.svc.RenameProject(r.PathValue("id"), req.Name)
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
