// Package harnesstest is a scriptable stand-in for a native harness, for tests
// of the service and server. it speaks only the harness interface.
package harnesstest

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"sync"

	"github.com/michaelquigley/mega/internal/harness"
	"github.com/michaelquigley/mega/internal/model"
)

// Harness launches fake runtimes and serves scripted history.
type Harness struct {
	name string

	mu         sync.Mutex
	runtimes   []*Runtime
	history    map[string][]*model.Event
	sessions   []harness.Session
	launchErr  error
	onLaunch   func()
	onHistory  func()
	onVersion  func()
	catalog    harness.Catalog
	settings   model.TurnSettings
	version    string
	versionErr error
	// configuration is where the fake keeps the operator's configuration
	// (SetConfiguration), nil for none.
	configuration func(env []string) []harness.ConfigRoot
	deferred      bool // runtimes take a model change with the next prompt, as codex does
	modelLists    int
	exitEarly     bool
	// duringLaunch runs with each new runtime before Launch returns, as an
	// adapter that reports to its sink while launching does.
	duringLaunch func(*Runtime)
	launched     int
	missingMu    sync.Mutex
	missing      error // what Check reports, under missingMu
	// program is the file the fake says it runs (SetProgram), empty for
	// none, under missingMu: Locate and Path are asked under the service's
	// lock, as Check is, so they never wait on a launch holding mu.
	program    string
	historyErr error    // what History fails with, if set
	reads      []string // native ids History was asked for, in order
	// places are where History, Discover, Models and Settings were asked to
	// run, in order: each checkout with the environment it was given.
	places []harness.Place
	// checked are the environments Check was given, under missingMu.
	checked [][]string
}

// Places returns where the helpers were asked to run, in order.
func (h *Harness) Places() []harness.Place {
	h.mu.Lock()
	defer h.mu.Unlock()
	return append([]harness.Place(nil), h.places...)
}

// Checked returns the environments Check was given, in order.
func (h *Harness) Checked() [][]string {
	h.missingMu.Lock()
	defer h.missingMu.Unlock()
	return append([][]string(nil), h.checked...)
}

// New returns a fake harness registered under name.
// New is a fake harness offering one model, fake-model, which takes the
// efforts low, medium and high, and settings that name the effort medium, so
// a new conversation copies a concrete request: fake-model, medium, and
// claude's manual mode. SetCatalog and SetSettings script others.
func New(name string) *Harness {
	return &Harness{
		name:    name,
		history: make(map[string][]*model.Event),
		catalog: harness.Catalog{Version: "fake", Models: []harness.ModelOption{
			{ID: DefaultModel, DisplayName: "Fake", Efforts: []string{"low", "medium", "high"}, IsDefault: true},
		}},
		settings: model.TurnSettings{Effort: DefaultEffort},
	}
}

// DefaultModel and DefaultEffort are what a new conversation copies from a
// fake harness nobody has scripted.
const (
	DefaultModel  = "fake-model"
	DefaultEffort = "medium"
)

func (h *Harness) Name() string { return h.name }

// FailLaunch makes the next launches fail with err; nil restores success.
func (h *Harness) FailLaunch(err error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.launchErr = err
}

// OnLaunch runs fn at the start of every launch, while the caller is inside
// Launch. fn must not call back into the service.
func (h *Harness) OnLaunch(fn func()) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.onLaunch = fn
}

// DuringLaunch runs fn with each new runtime before Launch returns, so a test
// can report through the sink as an adapter may while launching.
func (h *Harness) DuringLaunch(fn func(*Runtime)) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.duringLaunch = fn
}

// SetCatalog scripts what Models returns.
func (h *Harness) SetCatalog(c harness.Catalog) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.catalog = c
}

// SetSettings scripts what Settings returns: the operator's settings a new
// conversation copies.
func (h *Harness) SetSettings(settings model.TurnSettings) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.settings = settings
}

// SetVersion scripts what Version returns: the version its --version would
// print, or an error, as a build that cannot be read.
func (h *Harness) SetVersion(version string, err error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.version, h.versionErr = version, err
}

// SetProgram scripts the file the fake says it runs, for Locate and Path,
// as an adapter names the program it found.
func (h *Harness) SetProgram(path string) {
	h.missingMu.Lock()
	defer h.missingMu.Unlock()
	h.program = path
}

// Locate names the scripted program (SetProgram), empty for none.
func (h *Harness) Locate(_ []string) string {
	h.missingMu.Lock()
	defer h.missingMu.Unlock()
	if h.program == "" {
		return ""
	}
	return fmt.Sprintf("%s '%s'", h.name, h.program)
}

// Path is the scripted program (SetProgram), empty for none.
func (h *Harness) Path(_ []string) string {
	h.missingMu.Lock()
	defer h.missingMu.Unlock()
	return h.program
}

// SetConfiguration scripts where the fake's configuration is, as an
// adapter's Configuration answers from the operator's shell environment.
func (h *Harness) SetConfiguration(f func(env []string) []harness.ConfigRoot) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.configuration = f
}

// Configuration is the scripted configuration (SetConfiguration).
func (h *Harness) Configuration(env []string) []harness.ConfigRoot {
	h.mu.Lock()
	f := h.configuration
	h.mu.Unlock()
	if f == nil {
		return nil
	}
	return f(env)
}

// Invocation reads a tool call the fake recorded: a payload with a
// "command" is a shell command, any other map is its input. an event with no
// map payload holds nothing to read, as an action recorded with no payload.
func (h *Harness) Invocation(ev *model.Event) (harness.Invocation, bool) {
	input, ok := ev.Raw.(map[string]any)
	if !ok {
		return harness.Invocation{}, false
	}
	return harness.ShellInvocation(ev.Tool, input, "command"), true
}

// Version is the scripted version (SetVersion), as a harness's --version.
func (h *Harness) Version(_ context.Context, _ []string) (string, error) {
	h.mu.Lock()
	onVersion := h.onVersion
	h.mu.Unlock()
	if onVersion != nil {
		onVersion()
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.version, h.versionErr
}

// OnVersion runs fn each time the version is read, before it is returned, as
// the last thing the service does before a launch.
func (h *Harness) OnVersion(fn func()) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.onVersion = fn
}

// DeferModelChanges makes later runtimes take a model change with the next
// prompt (as codex does) rather than at once (as claude does).
func (h *Harness) DeferModelChanges(deferred bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.deferred = deferred
}

func (h *Harness) Models(_ context.Context, place harness.Place) (harness.Catalog, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.places = append(h.places, place)
	h.modelLists++
	return h.catalog, nil
}

// ModelLists counts the calls to Models.
func (h *Harness) ModelLists() int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.modelLists
}

// SetMissing makes Check report err, as a harness whose executable cannot be
// found does; nil makes it whole again.
func (h *Harness) SetMissing(err error) {
	h.missingMu.Lock()
	defer h.missingMu.Unlock()
	h.missing = err
}

// FailHistory makes History fail with err; nil makes it read again.
func (h *Harness) FailHistory(err error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.historyErr = err
}

// Check reports what SetMissing set. it takes its own lock, not h.mu: a
// workspace read runs it, and a test may read the workspace from inside
// Launch, which holds h.mu.
func (h *Harness) Check(env []string) error {
	h.missingMu.Lock()
	defer h.missingMu.Unlock()
	h.checked = append(h.checked, env)
	return h.missing
}

func (h *Harness) Settings(_ context.Context, place harness.Place) (model.TurnSettings, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.places = append(h.places, place)
	return h.settings, nil
}

// OnHistory runs fn at the start of every History call, before the history is
// read. fn may block, which holds the read open; it must not call back into the
// service.
func (h *Harness) OnHistory(fn func()) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.onHistory = fn
}

// SetHistory scripts what History returns for a native session.
func (h *Harness) SetHistory(nativeID string, evs []*model.Event) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.history[nativeID] = evs
}

// ExitDuringLaunch makes the next runtimes exit before Launch returns, as a
// runtime that reports its start and dies at once does.
func (h *Harness) ExitDuringLaunch(early bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.exitEarly = early
}

// SetSessions scripts what Discover returns.
func (h *Harness) SetSessions(sessions []harness.Session) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.sessions = sessions
}

func (h *Harness) Discover(_ context.Context, place harness.Place) ([]harness.Session, error) {
	h.mu.Lock()
	h.places = append(h.places, place)
	h.mu.Unlock()
	h.mu.Lock()
	defer h.mu.Unlock()
	return slices.Clone(h.sessions), nil
}

// Reads lists the native ids whose history was read.
func (h *Harness) Reads() []string {
	h.mu.Lock()
	defer h.mu.Unlock()
	return slices.Clone(h.reads)
}

// Runtimes lists every runtime launched, oldest first.
func (h *Harness) Runtimes() []*Runtime {
	h.mu.Lock()
	defer h.mu.Unlock()
	return slices.Clone(h.runtimes)
}

// Last is the newest runtime, or nil.
func (h *Harness) Last() *Runtime {
	h.mu.Lock()
	defer h.mu.Unlock()
	if len(h.runtimes) == 0 {
		return nil
	}
	return h.runtimes[len(h.runtimes)-1]
}

func (h *Harness) Launch(spec harness.Spec, sink harness.Sink) (harness.Runtime, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.onLaunch != nil {
		h.onLaunch()
	}
	if h.launchErr != nil {
		return nil, h.launchErr
	}
	h.launched++
	r := &Runtime{Spec: spec, sink: sink, nativeID: spec.NativeID, requests: make(map[string]string), deferred: h.deferred}
	if r.nativeID == "" {
		r.nativeID = fmt.Sprintf("native-%d", h.launched)
	}
	h.runtimes = append(h.runtimes, r)
	if h.exitEarly {
		r.Exit(errors.New("exited while starting"))
	}
	if h.duringLaunch != nil {
		h.duringLaunch(r)
	}
	return r, nil
}

func (h *Harness) History(_ context.Context, place harness.Place, nativeID string) (harness.History, error) {
	h.mu.Lock()
	onHistory := h.onHistory
	h.mu.Unlock()
	if onHistory != nil {
		onHistory()
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	h.reads = append(h.reads, nativeID)
	h.places = append(h.places, place)
	if h.historyErr != nil {
		return harness.History{}, h.historyErr
	}
	var out harness.History
	for _, ev := range h.history[nativeID] {
		copied := *ev
		out.Events = append(out.Events, &copied)
	}
	return out, nil
}

// Look is the look at a scripted history (SetHistory), as an adapter reads
// one through its own history; it counts as a read of that session.
func (h *Harness) Look(ctx context.Context, place harness.Place, nativeID string) (harness.Look, error) {
	history, err := h.History(ctx, place, nativeID)
	if err != nil {
		return harness.Look{}, err
	}
	return harness.LookOf(history.Events), nil
}

// Prompt is one prompt a fake runtime received.
type Prompt struct {
	UUID string
	Text string
}

// Delivery is one answer a fake runtime received, by its native request id.
type Delivery struct {
	Native string
	Answer harness.Answer
}

// Runtime is a fake runtime. its native request ids restart at 0 in every
// runtime, as codex's request numbers do, so tests can show the service never
// confuses requests across incarnations.
type Runtime struct {
	Spec harness.Spec
	sink harness.Sink

	mu           sync.Mutex
	nativeID     string
	prompts      []Prompt
	deliveries   []Delivery
	interrupts   int
	closed       bool
	exited       bool
	settled      bool // the sink has handled the exit
	holdOnClose  bool
	holdStops    bool   // an interrupt settles nothing, as pi's with a dialog pending
	fences       int    // stops fenced
	rejectWith   error  // an answer refused without settling the request
	settleAs     string // an answer settled through the sink with this outcome, returning settleErr
	settleErr    error
	refusePrompt error  // a prompt refused, no turn started
	holdStarts   bool   // a prompt declares no start; StartTurn does
	onAnswer     func() // runs while an answer is in flight, before it is delivered
	modes        []string
	models       []string // "model/effort" for each model change accepted
	deferred     bool
	modeErr      error
	onMode       func()
	afterMode    func()
	requests     map[string]string // service id -> native number
	nextNative   int
}

func (r *Runtime) NativeID() string { return r.nativeID }

// Prompt takes a prompt and, unless HoldStarts was set, starts its turn at
// once, as a harness declares the start of the turn a prompt begins.
func (r *Runtime) Prompt(uuid, text string) error {
	r.mu.Lock()
	if r.exited {
		r.mu.Unlock()
		return fmt.Errorf("runtime exited")
	}
	if r.refusePrompt != nil {
		r.mu.Unlock()
		return r.refusePrompt
	}
	r.prompts = append(r.prompts, Prompt{UUID: uuid, Text: text})
	hold := r.holdStarts
	r.mu.Unlock()
	if !hold {
		r.sink.TurnStarted()
	}
	return nil
}

// HoldStarts makes a prompt declare no start, as in the moment before a
// harness says its turn has begun, or a harness that never begins one; the
// test declares it with StartTurn.
func (r *Runtime) HoldStarts() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.holdStarts = true
}

// StartTurn declares a turn started, as a harness does for a prompt's turn
// or one it begins by itself.
func (r *Runtime) StartTurn() { r.sink.TurnStarted() }

func (r *Runtime) Answer(requestID string, answer harness.Answer) error {
	r.mu.Lock()
	hook := r.onAnswer
	r.mu.Unlock()
	if hook != nil {
		hook()
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	native, ok := r.requests[requestID]
	if !ok {
		return harness.ErrNotPending
	}
	if r.rejectWith != nil {
		return r.rejectWith
	}
	delete(r.requests, requestID)
	if r.settleAs != "" {
		outcome, err := r.settleAs, r.settleErr
		r.mu.Unlock()
		r.sink.Resolved(requestID, outcome)
		r.mu.Lock()
		if err == nil {
			r.deliveries = append(r.deliveries, Delivery{Native: native, Answer: answer})
		}
		return err
	}
	r.deliveries = append(r.deliveries, Delivery{Native: native, Answer: answer})
	return nil
}

// Interrupt aborts every pending request, as claude's abort signal does.
// SetPermissionMode switches like a real runtime: it runs the hook OnMode set,
// fails if FailMode said so, and otherwise records the switch and reports the
// confirmed mode to the sink as a mode change before returning. the hook
// AfterMode set runs after that report, still before the call returns.
func (r *Runtime) SetPermissionMode(mode string) error {
	r.mu.Lock()
	hook, err, after := r.onMode, r.modeErr, r.afterMode
	r.mu.Unlock()
	if hook != nil {
		hook()
	}
	if err != nil {
		return err
	}
	r.mu.Lock()
	r.modes = append(r.modes, mode)
	r.mu.Unlock()
	r.sink.Event(&model.Event{Kind: model.EventRuntime, Status: model.StatusModeChanged, Origin: model.OriginManaged, Text: "confirmed by claude; applies from the next tool call", Facts: &model.RuntimeFacts{PermissionMode: mode}})
	if after != nil {
		after()
	}
	return nil
}

// AfterMode runs fn inside every later successful switch, after its report and
// before it returns, as a newer report arriving first would. fn must not call
// back into the service.
func (r *Runtime) AfterMode(fn func()) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.afterMode = fn
}

// SetModel changes like a real runtime: it runs the hook OnMode set, fails
// if FailMode said so, and otherwise records the change. it reports nothing:
// the model a runtime runs is reported by Emit, as a harness reports it. it
// is deferred if the harness was told so.
func (r *Runtime) SetModel(model, effort string) (bool, error) {
	r.mu.Lock()
	hook, err := r.onMode, r.modeErr
	r.mu.Unlock()
	if hook != nil {
		hook()
	}
	if err != nil {
		return false, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.models = append(r.models, model+"/"+effort)
	return r.deferred, nil
}

// ModelChanges lists the model changes the runtime accepted, as "model/effort".
func (r *Runtime) ModelChanges() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return slices.Clone(r.models)
}

// FailMode makes later switches, of mode or model, fail with err; nil
// restores success.
func (r *Runtime) FailMode(err error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.modeErr = err
}

// OnMode runs fn inside every later switch, of mode or model, before it takes
// effect, while the caller waits. fn must not call back into the service.
func (r *Runtime) OnMode(fn func()) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.onMode = fn
}

// Modes lists the switches the runtime accepted, in order.
func (r *Runtime) Modes() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return slices.Clone(r.modes)
}

func (r *Runtime) Interrupt() error {
	r.mu.Lock()
	r.interrupts++
	if r.holdStops {
		r.mu.Unlock()
		return nil
	}
	var aborted []string
	for id := range r.requests {
		aborted = append(aborted, id)
	}
	clear(r.requests)
	r.mu.Unlock()
	slices.Sort(aborted)
	for _, id := range aborted {
		r.sink.Resolved(id, model.OutcomeAborted)
	}
	return nil
}

// Close ends input and, unless HoldOnClose was set, exits cleanly.
func (r *Runtime) Close() error {
	r.mu.Lock()
	r.closed = true
	hold := r.holdOnClose
	r.mu.Unlock()
	if !hold {
		r.Exit(nil)
	}
	return nil
}

// Stopping counts a stop fenced.
func (r *Runtime) Stopping() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.fences++
}

// Fences counts the stops fenced.
func (r *Runtime) Fences() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.fences
}

// RefusePrompts makes every prompt refused with err, as a harness refuses
// one it will not start; nil accepts again.
func (r *Runtime) RefusePrompts(err error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.refusePrompt = err
}

// RejectAnswers makes every answer refused with err, the request left
// pending, as a harness refuses an answer that does not fit; nil accepts again.
func (r *Runtime) RejectAnswers(err error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.rejectWith = err
}

// SettleAnswers makes every answer go as a codex steer does: the adapter
// settles the request itself, through the sink, with outcome, and the answer
// returns err, as nil for one codex confirmed or harness.ErrUnconfirmed for
// one it never did.
func (r *Runtime) SettleAnswers(outcome string, err error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.settleAs, r.settleErr = outcome, err
}

// HoldInterrupts makes an interrupt settle none of the pending requests, as
// pi's does while a dialog is pending: the request stays until it is answered.
func (r *Runtime) HoldInterrupts() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.holdStops = true
}

// OnAnswer runs fn while each answer is in flight, before it is delivered.
func (r *Runtime) OnAnswer(fn func()) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.onAnswer = fn
}

// HoldOnClose keeps the runtime alive after Close, like a backgrounded tool.
func (r *Runtime) HoldOnClose() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.holdOnClose = true
}

// Emit delivers an event as the harness would.
// Background reports the runtime's background jobs, as an adapter would.
func (r *Runtime) Background(jobs []model.BackgroundJob) { r.sink.Background(jobs) }

// MCPServers reports the runtime's MCP servers, as an adapter would.
func (r *Runtime) MCPServers(servers []model.MCPServer) { r.sink.MCPServers(servers) }

func (r *Runtime) Emit(ev *model.Event) {
	if ev.Origin == "" {
		ev.Origin = model.OriginManaged
	}
	r.sink.Event(ev)
}

// Delta delivers streaming text.
func (r *Runtime) Delta(text string) { r.sink.Delta(text) }

// Ask raises a request and returns the service's id for it.
func (r *Runtime) Ask(kind string, payload any) string {
	r.mu.Lock()
	defer r.mu.Unlock()
	native := strconv.Itoa(r.nextNative)
	r.nextNative++
	id := r.sink.Request(kind, payload)
	r.requests[id] = native
	return id
}

// Settle resolves a request on the harness side, as an abort would.
func (r *Runtime) Settle(id, outcome string) {
	r.mu.Lock()
	_, ok := r.requests[id]
	delete(r.requests, id)
	r.mu.Unlock()
	if ok {
		r.sink.Resolved(id, outcome)
	}
}

// Exit ends the runtime, once.
func (r *Runtime) Exit(err error) {
	r.mu.Lock()
	if r.exited {
		r.mu.Unlock()
		return
	}
	r.exited = true
	r.mu.Unlock()
	r.sink.Exited(err)
	// exited only once the sink has handled it: a test that waits on Exited
	// then sees the service settled, never the moment before it records the
	// stop.
	r.mu.Lock()
	r.settled = true
	r.mu.Unlock()
}

// Prompts lists the prompts received.
func (r *Runtime) Prompts() []Prompt {
	r.mu.Lock()
	defer r.mu.Unlock()
	return slices.Clone(r.prompts)
}

// Deliveries lists the answers received.
func (r *Runtime) Deliveries() []Delivery {
	r.mu.Lock()
	defer r.mu.Unlock()
	return slices.Clone(r.deliveries)
}

// Interrupts counts interrupts received.
func (r *Runtime) Interrupts() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.interrupts
}

// Closed reports whether input was closed.
func (r *Runtime) Closed() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.closed
}

// Exited reports whether the runtime has exited.
func (r *Runtime) Exited() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.settled
}
