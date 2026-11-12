// Package harness is the seam between the service and the native harness
// adapters. it speaks only model types: everything a harness's wire protocol
// looks like stays inside its adapter package.
package harness

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/michaelquigley/mega/internal/model"
)

// ErrNotPending reports an answer to a request the runtime no longer waits on.
var ErrNotPending = errors.New("request is not pending")

// ErrNotStarted marks a Launch failure the adapter can prove came before any
// native session could exist. adapters wrap it at each such failure site, never
// as a blanket over Launch: an error without it means the session is unknown,
// and the service then refuses to start a fresh one in its place.
var ErrNotStarted = errors.New("the runtime did not start")

// ErrModeUnconfirmed reports a permission mode switch the harness did not
// confirm in time: it may still take effect, and the runtime reports it as a
// mode change if it does.
var ErrModeUnconfirmed = errors.New("the permission mode switch was not confirmed in time")

// ErrSettingUnconfirmed reports a model or effort change the harness did
// not confirm in time.
var ErrSettingUnconfirmed = errors.New("the change was not confirmed in time")

// ErrEffortRefused reports a change whose model the harness accepted and
// whose effort it then refused: the model changed, the effort did not.
var ErrEffortRefused = errors.New("the model changed but the effort was refused")

// ErrLeftRunning reports a runtime whose own process exited while processes it
// started were still running when the bounded check gave up.
var ErrLeftRunning = errors.New("processes the runtime started are still running")

// Harness launches runtimes and reads native history for one kind of agent.
type Harness interface {
	// Name is the model.Harness* value conversations record.
	Name() string

	// Launch starts a runtime in cwd. an empty nativeID starts a new native
	// session; otherwise the runtime resumes exactly that session. the runtime's
	// native identity is known when Launch returns. Launch may deliver to the
	// sink before it returns, so callers must not hold a lock the sink needs.
	// a failure that provably came before any session could exist wraps
	// ErrNotStarted; any other failure leaves the session unknown.
	Launch(spec Spec, sink Sink) (Runtime, error)

	// History reads the native session's stored messages without a runtime.
	// every event carries a NativeID so the caller can match without text.
	History(ctx context.Context, place Place, nativeID string) (History, error)

	// Discover lists the native sessions stored for the place's checkout, for
	// the operator to choose from. it never picks one.
	Discover(ctx context.Context, place Place) ([]Session, error)

	// Models lists the models the harness offers, with the harness version
	// the list came from. it starts no conversation and leaves no session.
	Models(ctx context.Context, place Place) (Catalog, error)

	// Inherited says what model and effort a launch with no choice gets in
	// the place, from the operator's own settings, or empty where unknown.
	Inherited(ctx context.Context, place Place) (model, effort string, err error)

	// Check reports whether what the harness runs can be found in env, a
	// project's resolved environment, and in mega's configuration: nil, or a
	// *MissingError naming the first thing missing. it starts nothing and
	// reads only names and modes.
	Check(env []string) error
}

// Place is where a harness process runs on a project's behalf: the checkout,
// and the project's resolved environment, which every process the adapter
// starts gets in full, and nothing of mega's own.
type Place struct {
	Cwd string
	Env []string
}

// ErrExecutableMissing marks a harness that cannot run because something it
// executes cannot be found.
var ErrExecutableMissing = errors.New("an executable the harness runs is missing")

// MissingError names what a harness cannot find, and why.
type MissingError struct {
	Harness    string
	Executable string
	Reason     string
}

func (e *MissingError) Error() string {
	return fmt.Sprintf("'%s' cannot run: %s", e.Harness, e.Reason)
}

func (e *MissingError) Is(target error) bool {
	return target == ErrExecutableMissing
}

// FindExecutable checks that name can be run in env: a path is checked as
// given, a bare name is looked up on env's PATH, as a shell in that
// environment would.
func FindExecutable(harnessName, name string, env []string) error {
	if name == "" {
		return &MissingError{Harness: harnessName, Executable: name, Reason: "no executable is configured"}
	}
	if strings.Contains(name, "/") {
		info, err := os.Stat(name)
		switch {
		case err != nil:
			return &MissingError{Harness: harnessName, Executable: name, Reason: fmt.Sprintf("'%s' does not exist", name)}
		case info.IsDir() || info.Mode()&0o111 == 0:
			return &MissingError{Harness: harnessName, Executable: name, Reason: fmt.Sprintf("'%s' is not an executable file", name)}
		}
		return nil
	}
	if _, err := LookPath(name, env); err != nil {
		return &MissingError{Harness: harnessName, Executable: name, Reason: fmt.Sprintf("'%s' was not found on the project's PATH", name)}
	}
	return nil
}

// LookPath finds name as exec would, but on env's PATH rather than mega's own:
// a path is returned as it is, a bare name is searched for in each of env's
// PATH entries in order.
func LookPath(name string, env []string) (string, error) {
	if strings.Contains(name, "/") {
		return exec.LookPath(name)
	}
	var path string
	for _, kv := range env {
		if v, ok := strings.CutPrefix(kv, "PATH="); ok {
			path = v
		}
	}
	for _, dir := range filepath.SplitList(path) {
		if dir == "" || !filepath.IsAbs(dir) {
			continue
		}
		candidate := filepath.Join(dir, name)
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() && info.Mode()&0o111 != 0 {
			return candidate, nil
		}
	}
	return "", fmt.Errorf("'%s': %w", name, exec.ErrNotFound)
}

// Catalog is the models a harness offers.
type Catalog struct {
	Version string
	Models  []ModelOption
}

// ModelOption is one model a harness offers. Resolved is the id an alias
// runs as, where the harness says.
type ModelOption struct {
	ID            string
	Resolved      string
	DisplayName   string
	Description   string
	Efforts       []string
	DefaultEffort string
	IsDefault     bool
}

// History is a native session's stored messages, in order. Open names the
// events (by NativeID) that belong to a turn still in progress, so their output
// is not counted as finished; a harness whose history carries no turn status
// reports none.
type History struct {
	Events []*model.Event
	Open   map[string]bool
}

// Session is a native session found by discovery.
type Session struct {
	NativeID  string
	Preview   string // usually the first prompt
	CreatedAt time.Time
	UpdatedAt time.Time
}

// Spec describes a runtime to launch.
type Spec struct {
	Cwd string
	// Env is the project's resolved environment, which the runtime's process
	// gets in full and nothing of mega's own.
	Env      []string
	NativeID string
	// Model and Effort are the operator's choice; empty passes nothing, so
	// the harness's own settings apply.
	Model  string
	Effort string
	// PermissionMode is the operator's choice. empty asks the adapter for
	// mega's default: the mode the operator's native settings name,
	// inherited by passing none, else the manual mode.
	PermissionMode string
	// Idle is a launch with no prompt following it: the operator started the
	// runtime on its own. an adapter whose harness reports nothing until a
	// turn starts reports what it can without one.
	Idle bool
}

// Sink receives a runtime's output. calls arrive in order from one goroutine
// per runtime; a sink must not call back into the runtime while handling one.
type Sink interface {
	// Event delivers a normalized event to record.
	Event(ev *model.Event)

	// Delta delivers streaming text for liveness only; it is never recorded.
	Delta(text string)

	// Request records a question or approval and returns the service's id for
	// it. the adapter keeps the native id private and maps answers through it.
	Request(kind string, payload any) string

	// Resolved reports that the harness settled a request on its own, for
	// example by aborting it on interrupt.
	Resolved(requestID, outcome string)

	// Background reports the work the runtime keeps running outside a turn,
	// whole: each call replaces the last, and an empty list means none. it is
	// a fact for display, never recorded.
	Background(jobs []model.BackgroundJob)

	// Exited reports that the runtime process ended. err is nil for a clean
	// exit. it is the last call a sink receives.
	Exited(err error)
}

// Runtime is one running harness process for one conversation. its methods
// may block on process I/O, so callers must not hold locks a Sink needs.
type Runtime interface {
	// NativeID is the native session identity this runtime is bound to.
	NativeID() string

	// Prompt starts a turn. uuid is the service-minted message identity, which
	// the harness records so a later history import recognizes the prompt.
	Prompt(uuid, text string) error

	// Answer delivers the operator's answer. it returns ErrNotPending if the
	// harness already settled the request.
	Answer(requestID string, answer Answer) error

	// Interrupt stops the active turn, aborting any pending request.
	Interrupt() error

	// SetPermissionMode switches the running session's permission mode. it
	// returns once the harness confirms the switch, which applies from the next
	// tool call and leaves pending requests pending.
	SetPermissionMode(mode string) error

	// SetModel changes the model and effort for later responses, sending only
	// what differs from what the runtime last applied. deferred is true when
	// the harness takes the change with the next prompt rather than now.
	SetModel(model, effort string) (deferred bool, err error)

	// Close ends input. the runtime exits once the harness is idle.
	Close() error
}

// Answer is the operator's response to a request.
type Answer struct {
	Decision string            // "allow" | "deny", for a permission
	Message  string            // why, for a deny
	Answers  map[string]string // question text to chosen label, for a question
}

// decisions an Answer carries for a permission request.
const (
	DecisionAllow = "allow"
	DecisionDeny  = "deny"
	// DecisionAllowSession allows this request and later identical ones for
	// the rest of the native session, as the harness defines it. it is valid
	// only for a request whose payload offers it ("allow_session": true); the
	// service checks that before any harness sees it.
	DecisionAllowSession = "allow_session"
)
