// Package service is the single authority for workspace state. every write goes
// through it, and it is the only writer of the registry and the event logs.
package service

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"

	"github.com/michaelquigley/mega/internal/environ"
	"github.com/michaelquigley/mega/internal/harness"
	"github.com/michaelquigley/mega/internal/model"
	"github.com/michaelquigley/mega/internal/registry"
	"github.com/michaelquigley/mega/internal/snapshot"
)

// UntrustedChecker reports whether a checkout should carry the trust notice.
type UntrustedChecker func(checkout string) bool

// Options are the service's collaborators.
type Options struct {
	IsUntrusted UntrustedChecker
	// Resolver resolves each project's environment. everything mega starts
	// in a project's name runs in what it resolves, never in mega's own.
	Resolver     Resolver
	EventLogPath func(conversationID string) string
	Harnesses    []harness.Harness
	// Snapshots keeps copies of the harnesses' configuration (snapshot);
	// nil takes none, as the tests and the harness round trips do.
	Snapshots *snapshot.Store
	// ProjectsRoot is where the register dialog's folder picker starts and
	// what it confines its listing to (config projects_root); empty is the
	// home directory.
	ProjectsRoot string
	// SearchDirs are the directories beyond any checkout the reply box's @
	// search reaches (config path_search.directories), absolute. none keeps
	// the search to the checkout.
	SearchDirs []string
	// Exit ends the process at the first write to the record, the registry
	// or an event log, that fails, given why. it does not return. required.
	Exit func(reason error)
}

// Workspace is the arrangement as the service last confirmed it.
type Workspace struct {
	Projects      []model.Project
	Conversations []ConversationView

	// NeedsYou lists the conversations waiting on the operator, in the order
	// to take them: pending requests first, then unread output, oldest first.
	NeedsYou []string

	// Gate is the "shut down when idle" state and what is in its way.
	Gate Gate

	// Harnesses is whether each harness can run in the operator's shell
	// environment, as the start-up check found it, re-checked on every read;
	// nil until that check has run.
	Harnesses []HarnessStatus

	// Environments is each project's environment as it stands now, by project
	// id: its file, approval and override, and its latest resolution.
	Environments map[string]EnvState
}

// Service owns the registry, the event logs, the runtimes and their pending
// requests, and fans confirmed changes out to subscribers.
type Service struct {
	// failUpdate, when a test sets it, runs before each registry write and
	// fails it with what it returns.
	failUpdate func() error
	// failRecord, when a test sets it, runs before each event is appended and
	// fails the append with what it returns. it runs under s.mu.
	failRecord func(*model.Event) error
	// exit ends the process at a failed write (Options.Exit).
	exit func(reason error)
	// shutdownDone: an orderly Shutdown has finished, which Close announces.
	shutdownDone bool

	mu          sync.Mutex
	reg         *registry.Registry
	isUntrusted UntrustedChecker
	resolver    Resolver
	base        []string              // the scrubbed base environment, for reading the repository
	envs        map[string]*envRecord // each project's latest resolution, in memory only
	stopWatch   chan struct{}         // closed when the service closes, ending the file watch
	logPath     func(conversationID string) string
	harnesses   map[string]harness.Harness
	live        map[string]*live
	// closedLive holds the live records let go of at close, unchanged from
	// then: no runtime callback reaches them, and reads answered before the
	// HTTP server stops derive everything from their facts as ever.
	closedLive map[string]*live
	// logsClosed is set once Close has closed the logs: none is opened again,
	// so nothing past what was last confirmed can be read before a restart.
	logsClosed bool
	subs       map[*Subscription]struct{}
	closed     bool
	catalogs   catalogs      // model lists, per harness, for the service's life
	paths      pathCache     // checkout listings, held briefly
	outside    outsideSearch // the named directories' listings, held briefly
	// projectsRoot is where the folder picker starts (Folders); empty is the
	// home directory.
	projectsRoot string
	gate         shutdownGate
	// resolveSlots bounds the background resolutions running at once
	// (resolveAtOnce); background counts them, and the start-up check of the
	// harnesses, so a test can wait for them to settle.
	resolveSlots chan struct{}
	background   sync.WaitGroup
	// harnessFind is the environment the start-up check looked for the
	// harnesses in, and harnessWhere names it; nil until it has run.
	harnessFind  []string
	harnessWhere string
	// harnessVersions is each harness's version as last read, and
	// versionsReading says a read is under way (readVersions).
	harnessVersions map[string]versionRead
	versionsReading bool

	// snapshots is the configuration snapshot store, nil for none, and
	// snapshotting counts the snapshots under way, which Close waits for.
	snapshots    *snapshot.Store
	snapshotting sync.WaitGroup
}

// New returns a service over reg. it recovers from the previous run before
// returning: requests that were pending are resolved lost, and conversations
// whose runtime was not seen to exit become unsupervised.
func New(reg *registry.Registry, opts Options) (*Service, error) {
	if opts.Exit == nil {
		return nil, errors.New("the service needs a way to exit at a failed write")
	}
	s := &Service{
		exit:        opts.Exit,
		reg:         reg,
		isUntrusted: opts.IsUntrusted,
		logPath:     opts.EventLogPath,
		harnesses:   make(map[string]harness.Harness),
		live:        make(map[string]*live),
		envs:        make(map[string]*envRecord),
		resolver:    opts.Resolver,
		base:        environ.Base(os.Environ()),
		subs:        make(map[*Subscription]struct{}),
		gate:        shutdownGate{requested: make(chan struct{})},

		resolveSlots: make(chan struct{}, resolveAtOnce),
		snapshots:    opts.Snapshots,
		outside:      outsideSearch{dirs: opts.SearchDirs},
		projectsRoot: opts.ProjectsRoot,
	}
	if s.isUntrusted == nil {
		s.isUntrusted = func(string) bool { return false }
	}
	if s.resolver == nil {
		s.resolver = DefaultResolver(os.Environ())
	}
	for _, h := range opts.Harnesses {
		s.harnesses[h.Name()] = h
	}
	if err := s.recover(); err != nil {
		s.closeLogs()
		return nil, err
	}
	// conversations created before every one held a concrete request take
	// what their logs last reported, once.
	s.mu.Lock()
	s.fillChoicesLocked()
	// what each project and each harness is missing is found in the
	// background, so start-up and serving wait for neither.
	s.resolveAllLocked()
	s.mu.Unlock()
	s.background.Add(1)
	go func() {
		defer s.background.Done()
		s.checkHarnesses()
	}()
	s.stopWatch = make(chan struct{})
	go s.watchEnvFiles(envWatchEvery, s.stopWatch)
	return s, nil
}

// Workspace returns the current arrangement. trust notices are recomputed on
// each read, so a terminal visit clears the notice without a restart.
func (s *Service) Workspace() Workspace {
	s.mu.Lock()
	doc := s.reg.Snapshot()
	conversations := make([]ConversationView, 0, len(doc.Conversations))
	gate := Gate{Armed: s.gate.armed, ArmedAt: s.gate.armedAt}
	for _, conv := range doc.Conversations {
		view := s.viewLocked(conv)
		conversations = append(conversations, view)
		if b, ok := blockerOf(view, s.busyLocked(conv.ID)); ok {
			gate.Blockers = append(gate.Blockers, b)
		}
	}
	states := make(map[string]EnvState, len(doc.Projects))
	for i := range doc.Projects {
		s.applyTrustLocked(&doc.Projects[i])
		states[doc.Projects[i].ID] = s.envStateLocked(doc.Projects[i])
	}
	s.resolveStaleLocked(doc.Projects, states)
	harnesses := s.harnessStatusLocked()
	s.mu.Unlock()
	return Workspace{Projects: doc.Projects, Conversations: conversations, NeedsYou: needsYou(conversations), Gate: gate, Environments: states, Harnesses: harnesses}
}

// RegisterProject adds an existing checkout to the workspace. an empty name
// falls back to the checkout's directory name.
func (s *Service) RegisterProject(name, checkout string) (model.Project, error) {
	checkout, err := validateCheckout(checkout)
	if err != nil {
		return model.Project{}, err
	}
	name = strings.TrimSpace(name)
	if name == "" {
		name = filepath.Base(checkout)
	}
	project := model.Project{ID: registry.NewID(), Name: name, Checkout: checkout}

	s.mu.Lock()
	defer s.mu.Unlock()
	err = s.update(func(doc *registry.Document) error {
		for _, existing := range doc.Projects {
			if existing.Checkout == checkout {
				return newError(CodeConflict, fmt.Sprintf("checkout '%s' is already registered as '%s'", checkout, existing.Name))
			}
		}
		doc.Projects = append(doc.Projects, project)
		return nil
	})
	if err != nil {
		return model.Project{}, err
	}
	s.applyTrustLocked(&project)
	s.resolveRegisteredLocked(project)
	return project, nil
}

// RenameProject changes a project's display name.
func (s *Service) RenameProject(id, name string) (model.Project, error) {
	return s.UpdateProject(id, ProjectChange{Name: &name})
}

// ProjectChange is what an update of a project changes; a nil field is left
// as it is.
type ProjectChange struct {
	Name    *string
	Starred *bool
}

// UpdateProject changes a project's display name, its star, or both, in one
// registry write. a star is workspace state like the name: written only here,
// the same in every browser, and in memory only once the write is confirmed.
func (s *Service) UpdateProject(id string, change ProjectChange) (model.Project, error) {
	var name string
	if change.Name != nil {
		name = strings.TrimSpace(*change.Name)
		if name == "" {
			return model.Project{}, newError(CodeInvalid, "name is required")
		}
	}
	if change.Name == nil && change.Starred == nil {
		return model.Project{}, newError(CodeInvalid, "nothing to change: give a name or starred")
	}
	var project model.Project
	s.mu.Lock()
	defer s.mu.Unlock()
	err := s.update(func(doc *registry.Document) error {
		i := slices.IndexFunc(doc.Projects, func(p model.Project) bool { return p.ID == id })
		if i < 0 {
			return newError(CodeNotFound, fmt.Sprintf("no project '%s'", id))
		}
		if change.Name != nil {
			doc.Projects[i].Name = name
		}
		if change.Starred != nil {
			doc.Projects[i].Starred = *change.Starred
		}
		project = doc.Projects[i].Clone()
		return nil
	})
	if err != nil {
		return model.Project{}, err
	}
	s.applyTrustLocked(&project)
	return project, nil
}

// RemoveProject takes a project and its conversations out of the workspace,
// and deletes those conversations' event logs. it never touches the checkout
// or the native sessions, which discovery lists again if the checkout is
// registered again. it refuses while any of its conversations has a runtime in
// any state, as removing that conversation alone would. the project and its
// conversations leave the registry in one confirmed write, and only then are
// the logs deleted, in the same order and with the same failure handling as a
// single conversation's removal.
func (s *Service) RemoveProject(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, err := s.projectLocked(id); err != nil {
		return err
	}
	var removed []string
	for _, conv := range s.reg.Snapshot().Conversations {
		if conv.ProjectID != id {
			continue
		}
		if reason := s.busyReasonLocked(conv); reason != "" {
			return newError(CodeConflict, fmt.Sprintf("conversation '%s' %s and its project", conv.Name, reason))
		}
		removed = append(removed, conv.ID)
	}
	err := s.update(func(doc *registry.Document) error {
		i := slices.IndexFunc(doc.Projects, func(p model.Project) bool { return p.ID == id })
		if i < 0 {
			return newError(CodeNotFound, fmt.Sprintf("no project '%s'", id))
		}
		doc.Projects = slices.Delete(doc.Projects, i, i+1)
		doc.Conversations = slices.DeleteFunc(doc.Conversations, func(c model.Conversation) bool {
			return c.ProjectID == id
		})
		return nil
	})
	if err != nil {
		return err
	}
	s.dropLogsLocked(removed...)
	delete(s.envs, id)
	return nil
}

// Subscribe starts delivery of updates. after the service closes, the returned
// subscription's channel is already closed.
func (s *Service) Subscribe() *Subscription {
	sub := &Subscription{svc: s, updates: make(chan Update, updateBuffer)}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		close(sub.updates)
		return sub
	}
	s.subs[sub] = struct{}{}
	return sub
}

// update writes through the registry and announces the confirmed change.
// callers hold s.mu. it returns fn's own refusal, with nothing written, and
// refuses every write once the service has closed. a write that fails never
// returns: it ends the process (failed).
func (s *Service) update(fn func(*registry.Document) error) error {
	if s.logsClosed {
		// closed: the data directory may already be the next instance's, so
		// nothing more is written, whatever was still in flight.
		return newError(CodeUnavailable, "the service has closed; nothing more is written")
	}
	var refused error
	err := s.reg.Update(func(doc *registry.Document) error {
		if refused = fn(doc); refused != nil {
			return refused
		}
		if s.failUpdate != nil {
			return s.failUpdate()
		}
		return nil
	})
	if refused != nil {
		return refused
	}
	if err != nil {
		s.failed(err)
	}
	s.publish(Update{WorkspaceChanged: true})
	return nil
}

// failed is the one answer to a write to the record that failed: the process
// ends there, as a crash would leave it, and the next start settles what it
// left. nothing is stopped or told on the way out. it never returns.
func (s *Service) failed(err error) {
	s.exit(err)
	panic(fmt.Sprintf("the exit at a failed write returned: %v", err))
}

// publish delivers to every subscriber without blocking. callers hold s.mu, so
// subscribers see updates in the order they were confirmed.
func (s *Service) publish(update Update) {
	for sub := range s.subs {
		// each subscriber gets its own copy of an event, so none can change
		// what another, or the service, sees.
		own := update
		own.Event = update.Event.Clone()
		select {
		case sub.updates <- own:
		default:
			s.drop(sub)
		}
	}
}

// drop removes and closes a subscription. callers hold s.mu.
func (s *Service) drop(sub *Subscription) {
	if _, ok := s.subs[sub]; !ok {
		return
	}
	delete(s.subs, sub)
	close(sub.updates)
}

// applyTrustLocked fills what is read each time a project is read: whether
// its checkout can be worked in, and the trust notice from the operator's
// claude configuration. callers hold s.mu.
func (s *Service) applyTrustLocked(project *model.Project) {
	project.CheckoutMissing = checkoutProblem(project.Checkout)
	project.Untrusted = s.isUntrusted(project.Checkout)
}

func validateCheckout(checkout string) (string, error) {
	checkout = strings.TrimSpace(checkout)
	if checkout == "" {
		return "", newError(CodeInvalid, "checkout is required")
	}
	// ~ for the home directory, as the register dialog accepts it.
	if checkout == "~" || strings.HasPrefix(checkout, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			checkout = filepath.Join(home, strings.TrimPrefix(checkout, "~"))
		}
	}
	if !filepath.IsAbs(checkout) {
		return "", newError(CodeInvalid, fmt.Sprintf("checkout '%s' must be an absolute path", checkout))
	}
	checkout = filepath.Clean(checkout)
	info, err := os.Stat(checkout)
	if err != nil {
		return "", newError(CodeInvalid, fmt.Sprintf("checkout '%s' is not accessible here: %v", checkout, err))
	}
	if !info.IsDir() {
		return "", newError(CodeInvalid, fmt.Sprintf("checkout '%s' is not a directory", checkout))
	}
	return checkout, nil
}
