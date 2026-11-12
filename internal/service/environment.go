package service

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/michaelquigley/df/dl"

	"github.com/michaelquigley/mega/internal/environ"
	"github.com/michaelquigley/mega/internal/harness"
	"github.com/michaelquigley/mega/internal/model"
	"github.com/michaelquigley/mega/internal/registry"
)

// Resolver resolves a project's shell environment: the operator's shell run in
// the checkout from a scrubbed base, with the activation applied. the service
// starts nothing in a project's name without one.
type Resolver interface {
	Resolve(ctx context.Context, dir, activate string) (*environ.Resolution, error)
}

// DefaultResolver resolves through the operator's shell, named by SHELL in
// environ (mega's own environment, which names the operator and nothing about
// any project), from a base scrubbed of everything else. with no absolute
// SHELL it uses /bin/sh.
func DefaultResolver(env []string) *environ.Resolver {
	shell, _ := environ.Value(env, "SHELL")
	if !filepath.IsAbs(shell) {
		shell = "/bin/sh"
	}
	return &environ.Resolver{Shell: shell, Base: environ.Base(env), Timeout: environ.DefaultTimeout}
}

// helperFresh is how long a resolution serves the helpers (discovery, the
// model list, path search, an import) before they resolve again. a runtime
// start always resolves afresh.
const helperFresh = 15 * time.Second

// environment statuses, as the project page shows them.
const (
	EnvReady      = "ready"
	EnvUnapproved = "unapproved"
	EnvChanged    = "changed"
	EnvMalformed  = "malformed"
)

// EnvState is a project's environment as it stands now: the repository's file
// as read, what the operator approved and overrode, which activation would run
// and whether it may, and the latest resolution held in memory. it is
// computed on every read and never stored, apart from the approval and the
// override, which are workspace state.
type EnvState struct {
	File     environ.File
	Approved *model.EnvApproval
	Override *model.EnvOverride
	// Source is where the activation comes from: none, file, ignored or
	// override (model.EnvSource*).
	Source string
	// Activation fingerprints the activation that would run; empty with none.
	Activation string
	Status     string
	// Refusal is why work is refused here now, or nil.
	Refusal *Error
	// Resolved is the latest resolution in memory, or nil before any.
	Resolved *ResolvedEnv
	// Missing is what each harness cannot find in the latest resolution.
	Missing []harness.MissingError

	activate string
}

// ResolvedEnv is a resolution as the service holds it: when, from which
// activation, the names it changed and the fingerprint of the whole, or the
// failure. the variables themselves stay in memory, unexported, and are sent
// only when the operator asks for the changed ones' values.
type ResolvedEnv struct {
	At          time.Time
	Source      string
	Activation  string
	Changed     []string
	Fingerprint string
	Error       string

	env []string
}

// envRecord is the latest resolution of a project, and whether one runs now.
type envRecord struct {
	resolved *ResolvedEnv
	running  bool
}

// envStateLocked works out a project's environment from its checkout's file,
// its approval and its override. callers hold s.mu.
func (s *Service) envStateLocked(project model.Project) EnvState {
	st := EnvState{File: environ.ReadFile(project.Checkout), Approved: project.EnvApproved, Override: project.EnvOverride, Status: EnvReady}
	switch {
	case project.EnvOverride != nil && project.EnvOverride.Ignore:
		st.Source = model.EnvSourceIgnored
	case project.EnvOverride != nil:
		st.Source = model.EnvSourceOverride
		st.activate = project.EnvOverride.Activate
	case !st.File.Present:
		st.Source = model.EnvSourceNone
	case st.File.Malformed != "":
		st.Source = model.EnvSourceFile
		st.Status = EnvMalformed
		st.Refusal = &Error{Code: CodeEnvironmentInvalid, Message: fmt.Sprintf("%s; fix it, or set a local override on the project page", st.File.Malformed)}
	case project.EnvApproved == nil:
		st.Source = model.EnvSourceFile
		st.Status = EnvUnapproved
		st.Refusal = &Error{Code: CodeEnvironmentUnapproved, Message: fmt.Sprintf("'%s' has not been approved, so mega will not run its activation; review and approve it on the project page", st.File.Path)}
	case project.EnvApproved.Fingerprint != st.File.Fingerprint:
		st.Source = model.EnvSourceFile
		st.Status = EnvChanged
		st.Refusal = &Error{Code: CodeEnvironmentChanged, Message: fmt.Sprintf("'%s' has changed since it was approved, so mega will not run it; compare the two versions and approve the new one on the project page", st.File.Path)}
	default:
		st.Source = model.EnvSourceFile
		st.activate = st.File.Activate
	}
	if st.activate != "" {
		st.Activation = environ.Fingerprint([]byte(st.Source + "\x00" + st.activate))
	}
	if rec := s.envs[project.ID]; rec != nil && rec.resolved != nil {
		// a copy: the record stays the service's own.
		resolved := *rec.resolved
		resolved.Changed = slices.Clone(resolved.Changed)
		st.Resolved = &resolved
		if rec.resolved.Error == "" {
			st.Missing = s.preflightIn(rec.resolved.env)
		}
	}
	return st
}

// preflightIn checks every harness, in name order, in a resolved environment.
func (s *Service) preflightIn(env []string) []harness.MissingError {
	names := make([]string, 0, len(s.harnesses))
	for name := range s.harnesses {
		names = append(names, name)
	}
	slices.Sort(names)
	var missing []harness.MissingError
	for _, name := range names {
		var m *harness.MissingError
		if errors.As(s.harnesses[name].Check(env), &m) {
			missing = append(missing, *m)
		}
	}
	return missing
}

// envReadyLocked refuses work in a project whose environment may not run:
// its file is unapproved, changed or malformed. callers hold s.mu.
func (s *Service) envReadyLocked(project model.Project) error {
	if st := s.envStateLocked(project); st.Refusal != nil {
		return st.Refusal
	}
	return nil
}

// resolveProject resolves a project's environment afresh: the cheap refusals
// first, then the operator's shell in the checkout with the activation, if it
// may run. the result is held in memory as the project's latest resolution
// and returned with what a runtime start records of it, names and
// fingerprints only. a failed resolution is refused with
// CodeEnvironmentFailed: its message carries the activation's own output,
// for the operator, and what may be recorded does not. callers do not hold s.mu.
func (s *Service) resolveProject(projectID string) (harness.Place, *model.EnvironmentFacts, error) {
	s.mu.Lock()
	project, err := s.projectLocked(projectID)
	if err != nil {
		s.mu.Unlock()
		return harness.Place{}, nil, err
	}
	if err := s.checkoutLocked(project); err != nil {
		s.mu.Unlock()
		return harness.Place{}, nil, err
	}
	st := s.envStateLocked(project)
	if st.Refusal != nil {
		s.mu.Unlock()
		return harness.Place{}, nil, st.Refusal
	}
	rec := s.envs[projectID]
	if rec == nil {
		rec = &envRecord{}
		s.envs[projectID] = rec
	}
	rec.running = true
	s.mu.Unlock()

	res, resolveErr := s.resolver.Resolve(context.Background(), project.Checkout, st.activate)

	resolved := &ResolvedEnv{At: time.Now().UTC(), Source: st.Source, Activation: st.Activation}
	var failure error
	if resolveErr != nil {
		where := envWhere(st, project)
		var f *environ.Failure
		reason := resolveErr.Error()
		if errors.As(resolveErr, &f) {
			reason = f.Reason
		}
		resolved.Error = fmt.Sprintf("%s could not be resolved: %s", where, reason)
		failure = &Error{
			Code:     CodeEnvironmentFailed,
			Message:  fmt.Sprintf("%s could not be resolved, so nothing was started: %v", where, resolveErr),
			Recorded: fmt.Sprintf("%s could not be resolved: %s", where, reason),
		}
	} else {
		resolved.At, resolved.Changed, resolved.Fingerprint, resolved.env = res.At, slices.Clone(res.Changed), res.Fingerprint, res.Env
	}

	s.mu.Lock()
	rec.resolved, rec.running = resolved, false
	s.changed()
	s.mu.Unlock()
	if failure != nil {
		return harness.Place{}, nil, failure
	}
	return harness.Place{Cwd: project.Checkout, Env: res.Env}, &model.EnvironmentFacts{
		Source:      st.Source,
		Activation:  st.Activation,
		Changed:     res.Changed,
		Fingerprint: res.Fingerprint,
	}, nil
}

// envWhere names where an activation came from, for a message: the file's
// path, the local override, or the shell alone in the checkout.
func envWhere(st EnvState, project model.Project) string {
	switch st.Source {
	case model.EnvSourceFile:
		return fmt.Sprintf("the environment from '%s'", st.File.Path)
	case model.EnvSourceOverride:
		return fmt.Sprintf("the environment from the local override for '%s'", project.Checkout)
	default:
		return fmt.Sprintf("the shell environment in '%s'", project.Checkout)
	}
}

// placeFor is where a helper runs for a project: the latest resolution while
// it is fresh and from the activation that would run now, else a new one.
// callers do not hold s.mu.
func (s *Service) placeFor(projectID string) (harness.Place, error) {
	s.mu.Lock()
	project, err := s.projectLocked(projectID)
	if err == nil {
		st := s.envStateLocked(project)
		if st.Refusal != nil {
			s.mu.Unlock()
			return harness.Place{}, st.Refusal
		}
		if r := st.Resolved; r != nil && r.Error == "" && r.Activation == st.Activation && r.Source == st.Source && time.Since(r.At) < helperFresh {
			s.mu.Unlock()
			return harness.Place{Cwd: project.Checkout, Env: r.env}, nil
		}
	}
	s.mu.Unlock()
	place, _, err := s.resolveProject(projectID)
	return place, err
}

// resolveStaleLocked starts a resolution, in the background, for each project
// whose environment may run and whose latest resolution is missing or came
// from another activation, so the project page and the conversation headers
// compare against the environment a start would get now. a resolution that
// failed is not retried until the activation changes; a start, a helper or
// the operator's own request does that. callers hold s.mu.
func (s *Service) resolveStaleLocked(projects []model.Project, states map[string]EnvState) {
	for _, project := range projects {
		st := states[project.ID]
		if st.Refusal != nil || checkoutProblem(project.Checkout) != "" {
			continue
		}
		rec := s.envs[project.ID]
		if rec != nil && (rec.running || rec.resolved != nil && rec.resolved.Activation == st.Activation && rec.resolved.Source == st.Source) {
			continue
		}
		if rec == nil {
			s.envs[project.ID] = &envRecord{running: true}
		} else {
			rec.running = true
		}
		go func(id string) {
			if _, _, err := s.resolveProject(id); err != nil {
				// what may be recorded, never the activation's output.
				dl.Debugf("resolving the environment of '%s': %s", id, recordable(err))
			}
		}(project.ID)
	}
}

// ApproveEnvironment approves the checkout's .mega/environment.yaml at the
// fingerprint the operator was shown. a file that has changed since is
// refused, so an approval never covers content the operator did not see.
func (s *Service) ApproveEnvironment(projectID, fingerprint string) (model.Project, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	project, err := s.projectLocked(projectID)
	if err != nil {
		return model.Project{}, err
	}
	file := environ.ReadFile(project.Checkout)
	switch {
	case !file.Present:
		return model.Project{}, newError(CodeConflict, fmt.Sprintf("'%s' does not exist, so there is nothing to approve", file.Path))
	case file.Malformed != "":
		return model.Project{}, newError(CodeEnvironmentInvalid, file.Malformed)
	case strings.TrimSpace(fingerprint) == "":
		return model.Project{}, newError(CodeInvalid, "the fingerprint of the version being approved is required")
	case file.Fingerprint != fingerprint:
		return model.Project{}, newError(CodeConflict, fmt.Sprintf("'%s' has changed since it was shown (now '%s', shown '%s'); review it again", file.Path, file.Fingerprint, fingerprint))
	}
	approval := &model.EnvApproval{Fingerprint: file.Fingerprint, At: time.Now().UTC()}
	return s.changeProjectLocked(projectID, func(p *model.Project) { p.EnvApproved = approval })
}

// SetEnvironmentOverride sets the operator's local override for a project:
// ignore the repository's file, or run another activation in its place. nil
// clears it, and the file decides again.
func (s *Service) SetEnvironmentOverride(projectID string, override *model.EnvOverride) (model.Project, error) {
	if override != nil {
		// the service's own copy: the caller keeps no hold on workspace state.
		own := *override
		override = &own
		override.Activate = strings.TrimSpace(override.Activate)
		switch {
		case override.Ignore && override.Activate != "":
			return model.Project{}, newError(CodeInvalid, "an override either ignores the file or names an activation, not both")
		case !override.Ignore && override.Activate == "":
			return model.Project{}, newError(CodeInvalid, "an override needs an activation, or to ignore the file")
		}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, err := s.projectLocked(projectID); err != nil {
		return model.Project{}, err
	}
	return s.changeProjectLocked(projectID, func(p *model.Project) { p.EnvOverride = override })
}

// changeProjectLocked writes a change to one project through the registry
// and announces it once confirmed. callers hold s.mu.
func (s *Service) changeProjectLocked(projectID string, fn func(*model.Project)) (model.Project, error) {
	var changed model.Project
	err := s.update(func(doc *registry.Document) error {
		for i := range doc.Projects {
			if doc.Projects[i].ID == projectID {
				fn(&doc.Projects[i])
				changed = doc.Projects[i].Clone()
				return nil
			}
		}
		return newError(CodeNotFound, fmt.Sprintf("no project '%s'", projectID))
	})
	if err != nil {
		return model.Project{}, err
	}
	s.applyTrustLocked(&changed)
	return changed, nil
}

// EnvironmentValues is a live resolution's answer to the operator: when, the
// names the activation changed and the fingerprint, and, only when asked,
// the changed variables' values. it is sent to the asking page and nowhere
// else, and never stored.
type EnvironmentValues struct {
	At          time.Time
	Source      string
	Changed     []string
	Fingerprint string
	Values      map[string]string
}

// ResolveEnvironment resolves a project's environment now, at the operator's
// request, and returns the changed variables' values when values is true.
func (s *Service) ResolveEnvironment(projectID string, values bool) (EnvironmentValues, error) {
	place, facts, err := s.resolveProject(projectID)
	if err != nil {
		return EnvironmentValues{}, err
	}
	out := EnvironmentValues{Source: facts.Source, Changed: slices.Clone(facts.Changed), Fingerprint: facts.Fingerprint, At: time.Now().UTC()}
	s.mu.Lock()
	if rec := s.envs[projectID]; rec != nil && rec.resolved != nil {
		out.At = rec.resolved.At
	}
	s.mu.Unlock()
	if values {
		out.Values = make(map[string]string, len(facts.Changed))
		for _, name := range facts.Changed {
			if v, ok := environ.Value(place.Env, name); ok {
				out.Values[name] = v
			}
		}
	}
	return out, nil
}

// envStaleLocked says whether a runtime that started with facts would start
// differently now: the activation that would run is another, or the
// project's latest resolution has another fingerprint. callers hold s.mu.
func (s *Service) envStaleLocked(conv model.Conversation, facts *model.EnvironmentFacts) bool {
	if facts == nil {
		return false
	}
	project, err := s.projectLocked(conv.ProjectID)
	if err != nil {
		return false
	}
	st := s.envStateLocked(project)
	if st.Refusal != nil || st.Source != facts.Source || st.Activation != facts.Activation {
		return true
	}
	r := st.Resolved
	return r != nil && r.Error == "" && r.Activation == st.Activation && r.Fingerprint != facts.Fingerprint
}

// ApprovedVersion is the version of a project's .mega/environment.yaml the
// operator approved: its fingerprint and when, and, where the checkout's git
// history still has it, its content and where it was found. nothing but the
// fingerprint is stored, so a version that was never committed or staged
// cannot be shown.
type ApprovedVersion struct {
	Fingerprint string
	At          time.Time
	Available   bool
	Content     string
	Found       string
}

// ApprovedVersion finds the approved version of a project's file for the
// project page to show beside a changed one. git reads the repository in the
// scrubbed base environment, with no activation: the project's own cannot be
// resolved while its file waits on approval, and this only reads.
func (s *Service) ApprovedVersion(projectID string) (ApprovedVersion, error) {
	s.mu.Lock()
	project, err := s.projectLocked(projectID)
	s.mu.Unlock()
	if err != nil {
		return ApprovedVersion{}, err
	}
	if project.EnvApproved == nil {
		return ApprovedVersion{}, newError(CodeNotFound, fmt.Sprintf("nothing has been approved for '%s'", environ.ReadFile(project.Checkout).Path))
	}
	out := ApprovedVersion{Fingerprint: project.EnvApproved.Fingerprint, At: project.EnvApproved.At}
	if v, ok := environ.FindVersion(context.Background(), project.Checkout, out.Fingerprint, s.base); ok {
		out.Available, out.Content, out.Found = true, v.Content, v.Found
	}
	return out, nil
}

// envWatchEvery is how often the projects' environment files are looked at.
const envWatchEvery = 2 * time.Second

// watchEnvFiles announces a change whenever a project's .mega/environment.yaml
// appears, changes or goes, so the project page and the conversation headers
// follow an edit made in a terminal or by an agent without waiting for some
// other change. it reads names and content hashes only.
func (s *Service) watchEnvFiles(every time.Duration, stop <-chan struct{}) {
	seen := map[string]string{}
	ticker := time.NewTicker(every)
	defer ticker.Stop()
	for {
		select {
		case <-stop:
			return
		case <-ticker.C:
		}
		s.mu.Lock()
		projects := s.reg.Snapshot().Projects
		s.mu.Unlock()
		changed := false
		now := make(map[string]string, len(projects))
		for _, project := range projects {
			f := environ.ReadFile(project.Checkout)
			sig := fmt.Sprintf("%t:%s:%s", f.Present, f.Fingerprint, f.Malformed)
			now[project.ID] = sig
			// a project not looked at before counts as changed too: its file
			// may have changed before the first look.
			if old, ok := seen[project.ID]; !ok || old != sig {
				changed = true
			}
		}
		seen = now
		if changed {
			s.mu.Lock()
			if !s.closed {
				s.changed()
			}
			s.mu.Unlock()
		}
	}
}

// stopWatchLocked ends the file watch, once. callers hold s.mu.
func (s *Service) stopWatchLocked() {
	if s.stopWatch != nil {
		close(s.stopWatch)
		s.stopWatch = nil
	}
}

// envUnchangedLocked refuses a launch whose project's environment may no
// longer run, or would now run another activation than the one resolved for
// it (env), as when the file changed while it was being resolved. callers
// hold s.mu.
func (s *Service) envUnchangedLocked(project model.Project, env *model.EnvironmentFacts) error {
	st := s.envStateLocked(project)
	if st.Refusal != nil {
		return st.Refusal
	}
	if env != nil && (st.Source != env.Source || st.Activation != env.Activation) {
		path := st.File.Path
		return newError(CodeEnvironmentChanged, fmt.Sprintf("the environment for '%s' changed while it was being resolved (the file '%s' or the local override), so nothing was started; review it on the project page and start again", project.Checkout, path))
	}
	return nil
}
