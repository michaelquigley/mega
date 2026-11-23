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

// CarriesWork is whether an answer would let work go on: anything but a
// denial or a dismissal, which only settle a request toward a stop. once a
// stop has begun, the service refuses such an answer, and once it is fenced
// (Runtime.Stopping) no adapter delivers one, even one accepted before.
func CarriesWork(answer Answer) bool {
	return answer.Decision != DecisionDeny && answer.Decision != DecisionDismiss
}

// ErrPromptRefused reports a prompt the harness answered with a refusal: no
// turn started. a prompt whose fate is unknown, such as one that timed out,
// is not this.
var ErrPromptRefused = errors.New("the harness refused the prompt; no turn started")

// ErrUnconfirmed reports an answer that was sent and that the harness never
// confirmed: it may or may not have been taken. the adapter has settled the
// request as unconfirmed (model.OutcomeUnconfirmed) before returning it, and
// wraps it in what the operator is told, naming the harness.
var ErrUnconfirmed = errors.New("did not confirm whether it received your answer")

// ErrInvalidAnswer reports an answer the harness refused without settling the
// request, such as a choice it did not offer: the request is still pending,
// and a corrected answer or a dismissal still reaches it.
var ErrInvalidAnswer = errors.New("the answer does not fit the request")

// ErrWrongSession is what a read of a native session returned when it holds
// another session than the one asked for: a file found by its name, or a
// reply from the harness, is no proof of what it holds. nothing of it is
// imported.
var ErrWrongSession = errors.New("another session than the one asked for")

// SameSession refuses what a read of session want returned when it names
// another, got: what is the read, as a message names it. every adapter's
// history read and file read goes through it.
func SameSession(what, want, got string) error {
	if got == want {
		return nil
	}
	return fmt.Errorf("%w: %s holds session '%s', not '%s'", ErrWrongSession, what, got, want)
}

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

// ErrUnchecked reports a runtime whose own process exited where the processes
// it started could not be listed, as where /proc cannot be read: whether any
// outlived it is not known, which is neither a clean stop nor a leak.
var ErrUnchecked = errors.New("whether processes the runtime started are still running could not be checked")

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

	// Settings says what the operator's own settings name in the place: the
	// model, effort and (claude) permission mode, each empty where none is
	// named. a new conversation copies them once; the harness's settings never
	// reach a conversation after that.
	Settings(ctx context.Context, place Place) (model.TurnSettings, error)

	// Check reports whether what the harness runs can be found in find, the
	// environment mega locates its own programs in (a project's, before its
	// activation), and in mega's configuration: nil, or a *MissingError
	// naming the first thing missing. it starts nothing and reads only names
	// and modes.
	Check(find []string) error
}

// Locator is a harness that can say where the programs it runs were found in
// an environment it can run in: Locate names every program, for the
// start-up log, and Path the harness's own, for the first page.
type Locator interface {
	Locate(find []string) string
	Path(find []string) string
}

// Place is where a harness process runs on a project's behalf: the checkout;
// Env, the project's resolved environment, which every process the adapter
// starts gets in full, and nothing of mega's own; and Find, the environment
// before the project's activation, on whose PATH the adapter locates the
// programs it runs. an activation can change what an agent's commands see,
// but never which program mega itself executes.
type Place struct {
	Cwd  string
	Env  []string
	Find []string
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

// FindExecutable checks that name can be run from find: a path is checked as
// given, a bare name is looked up on find's PATH, and a script's
// #!/usr/bin/env interpreter is looked up the same way. advice, when given,
// is what to do about a configured path that cannot run, and ends its reason.
func FindExecutable(harnessName, name string, find []string, advice string) error {
	if name == "" {
		return &MissingError{Harness: harnessName, Executable: name, Reason: "no executable is configured"}
	}
	if strings.Contains(name, "/") {
		return CheckPath(harnessName, name, advice)
	}
	if _, err := Program(name, find); err != nil {
		var missing *MissingError
		if errors.As(err, &missing) {
			missing.Harness = harnessName
			return missing
		}
		return &MissingError{Harness: harnessName, Executable: name, Reason: fmt.Sprintf("'%s' was not found on the project's PATH", name)}
	}
	return nil
}

// CheckPath checks that a configured path is an executable file, as a
// harness's own check does before anything runs it: a path that does not
// exist, or is not an executable file, is a MissingError naming it, ending
// with advice, what the operator can do, when given.
func CheckPath(harnessName, path, advice string) error {
	suffix := ""
	if advice != "" {
		suffix = "; " + advice
	}
	info, err := os.Stat(path)
	switch {
	case err != nil:
		return &MissingError{Harness: harnessName, Executable: path, Reason: fmt.Sprintf("'%s' does not exist%s", path, suffix)}
	case info.IsDir() || info.Mode()&0o111 == 0:
		return &MissingError{Harness: harnessName, Executable: path, Reason: fmt.Sprintf("'%s' is not an executable file%s", path, suffix)}
	}
	return nil
}

// Program is how mega runs name, located in find: the whole command line,
// with every interpreter in a script's #! chain resolved through find and
// named explicitly, so neither the program nor any interpreter it leads to is
// whatever an activation put first on the PATH the process will see. env is
// followed in the forms "env PROG" and "env -S PROG ARGS", into find; any
// other form, env naming env, a relative interpreter, or a chain deeper than
// programDepth, is refused rather than run unresolved.
func Program(name string, find []string) ([]string, error) {
	return program(name, find, 0)
}

// programDepth bounds an interpreter chain, as the kernel bounds its own.
const programDepth = 4

func program(name string, find []string, depth int) ([]string, error) {
	if depth > programDepth {
		return nil, &MissingError{Executable: name, Reason: fmt.Sprintf("'%s' is reached through a chain of interpreters deeper than %d, so mega will not run it", name, programDepth)}
	}
	path, err := LookPath(name, find)
	if err != nil {
		return nil, &MissingError{Executable: name, Reason: fmt.Sprintf("'%s' was not found on the project's PATH", name)}
	}
	interp, rest, ok := shebang(path)
	if !ok {
		return []string{path}, nil
	}
	refuse := func() ([]string, error) {
		return nil, &MissingError{Executable: name, Reason: fmt.Sprintf("'%s' names its interpreter as '%s', a form mega does not follow, so it will not run it; name the interpreter directly", path, strings.TrimSpace(interp+" "+rest))}
	}
	if !filepath.IsAbs(interp) {
		return refuse()
	}
	var next string
	var args []string
	if filepath.Base(interp) == "env" {
		command := strings.Fields(rest)
		switch {
		case len(command) > 0 && command[0] == "-S":
			command = command[1:]
		case len(command) != 1:
			// without -S the kernel hands env the rest as one word.
			return refuse()
		}
		if len(command) == 0 || strings.HasPrefix(command[0], "-") || strings.Contains(command[0], "=") || filepath.Base(command[0]) == "env" {
			return refuse()
		}
		next, args = command[0], command[1:]
	} else {
		// an absolute interpreter gets the rest of the line as one argument,
		// as the kernel gives it; it may itself be a script, followed too.
		next = interp
		if rest != "" {
			args = []string{rest}
		}
	}
	inner, err := program(next, find, depth+1)
	if err != nil {
		var missing *MissingError
		if errors.As(err, &missing) && missing.Executable == next {
			missing.Reason = fmt.Sprintf("'%s', the interpreter '%s' names, cannot be run: %s", next, path, missing.Reason)
		}
		return nil, err
	}
	return append(append(inner, args...), path), nil
}

// shebang reads a script's #! line: its interpreter and the rest of the line.
// ok is false for anything that does not start with #!.
func shebang(path string) (interp, rest string, ok bool) {
	f, err := os.Open(path)
	if err != nil {
		return "", "", false
	}
	defer f.Close()
	head := make([]byte, 256)
	n, _ := f.Read(head)
	line, _, _ := strings.Cut(string(head[:n]), "\n")
	body, found := strings.CutPrefix(line, "#!")
	if !found {
		return "", "", false
	}
	body = strings.TrimSpace(body)
	interp, rest, _ = strings.Cut(body, " ")
	return interp, strings.TrimSpace(rest), interp != ""
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

// History is a native session's stored messages, in order.
type History struct {
	Events []*model.Event
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
	// gets in full and nothing of mega's own. Find is the environment before
	// the activation, where the adapter locates the programs it runs.
	Env      []string
	Find     []string
	NativeID string
	// Model, Effort and PermissionMode are what the operator requests for
	// the conversation, always concrete, and always passed: the harness's own
	// settings never choose them. Effort is empty only for a model that takes
	// no effort level, and PermissionMode for a harness with none.
	Model          string
	Effort         string
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

	// MCPServers reports the conversation's MCP servers, whole: each call
	// replaces the last. it is a fact for display, never recorded and never
	// attention.
	MCPServers(servers []model.MCPServer)

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

	// Stopping tells the runtime a stop has begun, before the interrupt or the
	// close that carries it out: from now on nothing it is asked to deliver
	// starts work. it returns at once, never waiting on the harness, and is
	// the one fence every stop puts up first.
	Stopping()

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
	// DecisionDismiss dismisses a question without answering it, as Escape
	// does in the harness's own terminal: pi's own cancel. it is valid only
	// for a request whose payload offers it ("dismiss": true), and only the
	// operator sends it; mega never dismisses a dialog itself.
	DecisionDismiss = "dismiss"
)
