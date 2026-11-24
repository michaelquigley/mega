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
	// CheckoutMissing says why the checkout cannot be worked in ("does not
	// exist", "not a directory", "not readable"); absent when it can.
	CheckoutMissing string `dd:",+omitempty"`
	// Environment is the project's environment as it stands now; it comes
	// with the workspace read.
	Environment *environmentView `dd:",+omitempty"`
}

func newProjectView(project model.Project) *projectView {
	return &projectView{
		ID:              project.ID,
		Name:            project.Name,
		Checkout:        project.Checkout,
		Starred:         project.Starred,
		Untrusted:       project.Untrusted,
		CheckoutMissing: project.CheckoutMissing,
	}
}

type workspaceView struct {
	Projects      []*projectView
	Conversations []*conversationView
	NeedsYou      []string
	Shutdown      *shutdownView
	// Harnesses is whether each harness can run in the operator's shell
	// environment, as the start-up check found it; absent until it has run.
	Harnesses []*harnessStatusView `dd:",+omitempty"`
	// TimeZone is the zone the service runs in (timeZone), which the page
	// formats every time in; absent when it cannot be named.
	TimeZone string `dd:",+omitempty"`
}

// harnessStatusView is one harness, found where or missing and why, with its
// version against its tested range once read.
type harnessStatusView struct {
	Name     string
	Found    bool
	Where    string `dd:",+omitempty"`
	Path     string `dd:",+omitempty"`
	Reason   string `dd:",+omitempty"`
	Version  string `dd:",+omitempty"`
	Tested   string `dd:",+omitempty"`
	Standing string `dd:",+omitempty"`
	Note     string `dd:",+omitempty"`
}

func newHarnessStatusView(h service.HarnessStatus) *harnessStatusView {
	return &harnessStatusView{Name: h.Name, Found: h.Found, Where: h.Where, Path: h.Path, Reason: h.Reason, Version: h.Version, Tested: h.Tested, Standing: h.Standing, Note: h.Note}
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

// foldersView is one folder as the register dialog's picker shows it
// (service.Folders): its folders by name, each marked a git checkout or
// registered, the way up (absent at the projects root), and whether the
// listing stopped at its bound.
type foldersView struct {
	Path    string
	Full    string
	Parent  string `dd:",+omitempty"`
	Root    string
	Entries []*folderView
	Stopped bool
	Note    string `dd:",+omitempty"`
}

type folderView struct {
	Name       string
	Git        bool
	Registered string `dd:",+omitempty"`
}

// folders lists a folder inside the projects root for the register dialog's
// picker; path empty is the root itself.
func (s *Server) folders(w http.ResponseWriter, r *http.Request) {
	found, err := s.svc.Folders(r.URL.Query().Get("path"))
	if err != nil {
		writeError(w, r, err)
		return
	}
	view := &foldersView{Path: found.Path, Full: found.Full, Parent: found.Parent, Root: found.Root, Stopped: found.Stopped, Note: found.Note, Entries: make([]*folderView, 0, len(found.Entries))}
	for _, f := range found.Entries {
		view.Entries = append(view.Entries, &folderView{Name: f.Name, Git: f.Git, Registered: f.Registered})
	}
	writeJSON(w, r, http.StatusOK, view)
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
	w.Header().Set(BuildHeader, s.about.Interface)
	workspace, err := s.svc.Workspace()
	if err != nil {
		writeError(w, r, err)
		return
	}
	view := &workspaceView{
		Projects:      make([]*projectView, 0, len(workspace.Projects)),
		Conversations: make([]*conversationView, 0, len(workspace.Conversations)),
		NeedsYou:      workspace.NeedsYou,
		TimeZone:      s.zone,
		Shutdown: &shutdownView{
			Armed:    workspace.Gate.Armed,
			ArmedAt:  workspace.Gate.ArmedAt,
			Idle:     workspace.Gate.Idle(),
			Blockers: make([]*blockerView, 0, len(workspace.Gate.Blockers)),
		},
	}
	for _, h := range workspace.Harnesses {
		view.Harnesses = append(view.Harnesses, newHarnessStatusView(h))
	}
	for _, b := range workspace.Gate.Blockers {
		view.Shutdown.Blockers = append(view.Shutdown.Blockers, &blockerView{ConversationID: b.ConversationID, ProjectID: b.ProjectID, Name: b.Name, Turn: b.Turn, Requests: b.Requests, Background: b.Background, Settling: b.Settling})
	}
	for _, project := range workspace.Projects {
		pv := newProjectView(project)
		if st, ok := workspace.Environments[project.ID]; ok {
			pv.Environment = newEnvironmentView(st)
		}
		view.Projects = append(view.Projects, pv)
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

// pathsView is a path search's answer: the paths as the list shows them,
// relative to the checkout, or, for a search of the directories named beyond
// it, with ~ for the home directory; then Full, for those only, each path's
// absolute form in the same order, for a harness that is given it; and
// Stopped, each named directory whose listing stopped at its bound, so the
// list can say a path missing from it may only be unseen.
type pathsView struct {
	Paths   []string
	Full    []string `dd:",+omitempty"`
	Stopped []string `dd:",+omitempty"`
}

// paths searches a project's checkout by name, or the directories the
// operator named beyond it for a query starting with ~/ or /. directories
// end in a slash.
func (s *Server) paths(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query().Get("q")
	outside, searched, err := s.svc.OutsidePaths(r.PathValue("id"), q)
	if err != nil {
		writeError(w, r, err)
		return
	}
	if searched {
		view := &pathsView{Paths: make([]string, 0, len(outside.Paths)), Full: make([]string, 0, len(outside.Paths)), Stopped: outside.Stopped}
		for _, p := range outside.Paths {
			view.Paths = append(view.Paths, p.Shown)
			view.Full = append(view.Full, p.Full)
		}
		writeJSON(w, r, http.StatusOK, view)
		return
	}
	paths, err := s.svc.Paths(r.PathValue("id"), q)
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeJSON(w, r, http.StatusOK, &pathsView{Paths: paths})
}
