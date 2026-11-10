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

	"git.hq.quigley.com/research/roundhouse/internal/harness"
	"git.hq.quigley.com/research/roundhouse/internal/model"
)

// Harness launches fake runtimes and serves scripted history.
type Harness struct {
	name string

	mu        sync.Mutex
	runtimes  []*Runtime
	history   map[string][]*model.Event
	open      map[string]map[string]bool
	sessions  []harness.Session
	launchErr error
	onLaunch  func()
	onHistory func()
	exitEarly bool
	launched  int
	reads     []string // native ids History was asked for, in order
}

// New returns a fake harness registered under name.
func New(name string) *Harness {
	return &Harness{name: name, history: make(map[string][]*model.Event), open: make(map[string]map[string]bool)}
}

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

func (h *Harness) Discover(context.Context, string) ([]harness.Session, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	return slices.Clone(h.sessions), nil
}

// SetOpen marks events of a native session's history as belonging to a turn
// still in progress.
func (h *Harness) SetOpen(nativeID string, eventIDs ...string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	open := make(map[string]bool)
	for _, id := range eventIDs {
		open[id] = true
	}
	h.open[nativeID] = open
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
	r := &Runtime{Spec: spec, sink: sink, nativeID: spec.NativeID, requests: make(map[string]string)}
	if r.nativeID == "" {
		r.nativeID = fmt.Sprintf("native-%d", h.launched)
	}
	h.runtimes = append(h.runtimes, r)
	if h.exitEarly {
		r.Exit(errors.New("exited while starting"))
	}
	return r, nil
}

func (h *Harness) History(_ context.Context, _ string, nativeID string) (harness.History, error) {
	h.mu.Lock()
	onHistory := h.onHistory
	h.mu.Unlock()
	if onHistory != nil {
		onHistory()
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	h.reads = append(h.reads, nativeID)
	out := harness.History{Open: h.open[nativeID]}
	for _, ev := range h.history[nativeID] {
		copied := *ev
		out.Events = append(out.Events, &copied)
	}
	return out, nil
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

	mu          sync.Mutex
	nativeID    string
	prompts     []Prompt
	deliveries  []Delivery
	interrupts  int
	closed      bool
	exited      bool
	holdOnClose bool
	modes       []string
	modeErr     error
	onMode      func()
	afterMode   func()
	requests    map[string]string // service id -> native number
	nextNative  int
}

func (r *Runtime) NativeID() string { return r.nativeID }

func (r *Runtime) Prompt(uuid, text string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.exited {
		return fmt.Errorf("runtime exited")
	}
	r.prompts = append(r.prompts, Prompt{UUID: uuid, Text: text})
	return nil
}

func (r *Runtime) Answer(requestID string, answer harness.Answer) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	native, ok := r.requests[requestID]
	if !ok {
		return harness.ErrNotPending
	}
	delete(r.requests, requestID)
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

// FailMode makes later switches fail with err; nil restores success.
func (r *Runtime) FailMode(err error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.modeErr = err
}

// OnMode runs fn inside every later switch, before it takes effect, while
// the caller waits. fn must not call back into the service.
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

// HoldOnClose keeps the runtime alive after Close, like a backgrounded tool.
func (r *Runtime) HoldOnClose() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.holdOnClose = true
}

// Emit delivers an event as the harness would.
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
	return r.exited
}
