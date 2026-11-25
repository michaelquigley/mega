package server

import (
	"net/http"
	"time"

	"github.com/michaelquigley/mega/internal/environ"
	"github.com/michaelquigley/mega/internal/model"
	"github.com/michaelquigley/mega/internal/service"
)

// environmentView is a project's environment on the wire: the repository's
// file as read now, the operator's approval (a fingerprint and when) and
// override, which activation applies and whether work may start, the latest
// resolution (names and a fingerprint, never values), and what each harness
// cannot find in it.
type environmentView struct {
	File     *envFileView
	Approved *envApprovedView `dd:",+omitempty"`
	Override *envOverrideView `dd:",+omitempty"`
	Source   string
	Status   string
	Refusal  *errorBody       `dd:",+omitempty"`
	Resolved *envResolvedView `dd:",+omitempty"`
	Missing  []*missingView
}

type envFileView struct {
	Path    string
	Present bool
	Content string `dd:",+omitempty"`
	// Digest is the full sha256 an approval binds; Fingerprint its short form
	// for display.
	Digest      string    `dd:",+omitempty"`
	Fingerprint string    `dd:",+omitempty"`
	ModifiedAt  time.Time `dd:",+omitempty"`
	Malformed   string    `dd:",+omitempty"`
}

// envApprovedView is the approval: its digest (empty for one stored in the
// old short form, which no longer counts) and short fingerprint, when, and the
// names its activation changed then, once known.
type envApprovedView struct {
	Digest      string `dd:",+omitempty"`
	Fingerprint string
	At          time.Time
	Changed     []string
	NamesKnown  bool
}

type envOverrideView struct {
	Ignore   bool
	Activate string `dd:",+omitempty"`
}

type envResolvedView struct {
	At          time.Time
	Source      string
	Changed     []string
	Fingerprint string `dd:",+omitempty"`
	Error       string `dd:",+omitempty"`
	// Added and Removed are the names it changed beyond, and short of, the
	// approval's, when they differ.
	Added   []string `dd:",+omitempty"`
	Removed []string `dd:",+omitempty"`
}

type missingView struct {
	Harness    string
	Executable string
	Reason     string
}

func newEnvironmentView(st service.EnvState) *environmentView {
	view := &environmentView{
		File: &envFileView{
			Path:        st.File.Path,
			Present:     st.File.Present,
			Content:     st.File.Content,
			Digest:      st.File.Digest,
			Fingerprint: st.File.Fingerprint,
			ModifiedAt:  st.File.ModifiedAt,
			Malformed:   st.File.Malformed,
		},
		Source:  st.Source,
		Status:  st.Status,
		Missing: make([]*missingView, 0, len(st.Missing)),
	}
	if a := st.Approved; a != nil {
		short := environ.Short(a.Digest)
		if a.Digest == "" {
			short = a.Fingerprint
		}
		view.Approved = &envApprovedView{Digest: a.Digest, Fingerprint: short, At: a.At, Changed: append([]string{}, a.Changed...), NamesKnown: a.NamesKnown}
	}
	if st.Override != nil {
		view.Override = &envOverrideView{Ignore: st.Override.Ignore, Activate: st.Override.Activate}
	}
	if st.Refusal != nil {
		view.Refusal = &errorBody{Code: st.Refusal.Code, Message: st.Refusal.Message}
	}
	if r := st.Resolved; r != nil {
		view.Resolved = &envResolvedView{At: r.At, Source: r.Source, Changed: append([]string{}, r.Changed...), Fingerprint: r.Fingerprint, Error: r.Error, Added: r.Added, Removed: r.Removed}
	}
	for _, m := range st.Missing {
		view.Missing = append(view.Missing, &missingView{Harness: m.Harness, Executable: m.Executable, Reason: m.Error()})
	}
	return view
}

type approveEnvironmentRequest struct {
	Digest string
}

// approveEnvironment approves the checkout's .mega/environment.yaml at the
// full digest of the version the page showed.
func (s *Server) approveEnvironment(w http.ResponseWriter, r *http.Request) {
	var req approveEnvironmentRequest
	if !readJSON(w, r, &req) {
		return
	}
	project, err := s.svc.ApproveEnvironment(r.PathValue("id"), req.Digest)
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeJSON(w, r, http.StatusOK, newProjectView(project))
}

type overrideRequest struct {
	Ignore   bool
	Activate string
}

// setEnvironmentOverride sets the operator's local override for a project.
func (s *Server) setEnvironmentOverride(w http.ResponseWriter, r *http.Request) {
	var req overrideRequest
	if !readJSON(w, r, &req) {
		return
	}
	project, err := s.svc.SetEnvironmentOverride(r.PathValue("id"), &model.EnvOverride{Ignore: req.Ignore, Activate: req.Activate})
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeJSON(w, r, http.StatusOK, newProjectView(project))
}

// clearEnvironmentOverride removes the override, so the file decides again.
func (s *Server) clearEnvironmentOverride(w http.ResponseWriter, r *http.Request) {
	project, err := s.svc.SetEnvironmentOverride(r.PathValue("id"), nil)
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeJSON(w, r, http.StatusOK, newProjectView(project))
}

// resolvedValuesView is a live resolution: names and a fingerprint, and the
// changed variables' values only when values=1 asked for them. it goes to the
// page that asked and nowhere else.
type resolvedValuesView struct {
	At          time.Time
	Source      string
	Changed     []string
	Fingerprint string
	Values      map[string]string `dd:",+omitempty"`
}

// resolveEnvironment resolves a project's environment now, at the operator's
// request. it runs the activation, so it is a POST.
func (s *Server) resolveEnvironment(w http.ResponseWriter, r *http.Request) {
	values := r.URL.Query().Get("values") == "1"
	res, err := s.svc.ResolveEnvironment(r.PathValue("id"), values)
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeJSON(w, r, http.StatusOK, &resolvedValuesView{At: res.At, Source: res.Source, Changed: append([]string{}, res.Changed...), Fingerprint: res.Fingerprint, Values: res.Values})
}

type approvedVersionView struct {
	Digest      string `dd:",+omitempty"`
	Fingerprint string `dd:",+omitempty"`
	At          time.Time
	Available   bool
	Content     string `dd:",+omitempty"`
	Found       string `dd:",+omitempty"`
}

// approvedVersion finds the approved version of the file in the checkout's
// git history, to show beside a changed one.
func (s *Server) approvedVersion(w http.ResponseWriter, r *http.Request) {
	v, err := s.svc.ApprovedVersion(r.PathValue("id"))
	if err != nil {
		writeError(w, r, err)
		return
	}
	writeJSON(w, r, http.StatusOK, &approvedVersionView{Digest: v.Digest, Fingerprint: environ.Short(v.Digest), At: v.At, Available: v.Available, Content: v.Content, Found: v.Found})
}
