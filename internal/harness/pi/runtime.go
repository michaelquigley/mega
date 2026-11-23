package pi

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/michaelquigley/mega/internal/harness"
	"github.com/michaelquigley/mega/internal/model"
)

const (
	// callTimeout bounds a command's response; a turn runs as long as it runs.
	callTimeout = time.Minute
	// politeGrace is how long a closed pi gets to exit on its own while a
	// dialog is pending, and signalGrace how long each signal after it gets.
	// a pending dialog holds pi open through its input closing and a first
	// SIGTERM; a second ends it at once (pi 1.5.2, capability report,
	// 2026-11-16).
	politeGrace = 3 * time.Second
	signalGrace = 3 * time.Second
	// idleGrace is how long a closed pi with no dialog pending gets to finish
	// what it is doing before the same signals: pi waits for pending work.
	idleGrace = 30 * time.Second
	// survivorGrace is how long the exit waits for what pi started to go
	// before naming it still running.
	survivorGrace = 5 * time.Second
)

// dialog methods pi raises as extension_ui_request records and waits on; the
// others (notify, setStatus, setWidget, setTitle, set_editor_text) expect
// no answer (pi 1.5.2, docs/rpc-extension-ui.md).
var dialogMethods = []string{"select", "confirm", "input", "editor"}

// dialog is a pending extension dialog, by pi's own id, which never leaves
// this package.
type dialog struct {
	native  string
	method  string
	title   string
	options []string
	timer   *time.Timer
	// deadline is when pi settles it on its own, for a dialog with a
	// timeout; zero for one without.
	deadline time.Time
}

type runtime struct {
	conn *conn
	sink harness.Sink
	// stopping latches a fenced stop: no answer that would let work go on
	// is delivered after it (harness.CarriesWork).
	stopping bool

	// frames carries what pi sends, in order, from the connection's reader
	// to work, which handles them one at a time. worked closes when work
	// is done. discard closes when
	// a launch failed, so nothing waits on frames nobody reads.
	frames  chan received
	worked  chan struct{}
	discard chan struct{}

	mu         sync.Mutex
	sessionID  string
	dialogs    map[string]*dialog // service request id -> dialog
	turnActive bool
	lastStop   string // the last assistant message's stop reason in this run
	lastError  string // and its error
	model      string // the conversation's model and effort, as last applied
	effort     string
	closing    bool
	closedAt   time.Time
	escalated  bool // a signal had to end pi
}

func newRuntime(sink harness.Sink) *runtime {
	return &runtime{
		sink:    sink,
		dialogs: make(map[string]*dialog),
		frames:  make(chan received, 4096),
		worked:  make(chan struct{}),
		discard: make(chan struct{}),
	}
}

// received is a record pi sent, with when it arrived: a dialog's timeout
// runs from when pi sent it, not from when it is handled.
type received struct {
	f  map[string]any
	at time.Time
}

// frame queues one record pi sent besides a response, for work.
func (r *runtime) frame(f map[string]any) {
	select {
	case r.frames <- received{f: f, at: time.Now()}:
	case <-r.discard:
	}
}

// work handles what pi sent, in order, until the process is gone.
func (r *runtime) work() {
	defer close(r.worked)
	for rec := range r.frames {
		r.handle(rec.f, rec.at)
	}
}

func (r *runtime) NativeID() string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.sessionID
}

// Prompt sends a prompt. pi has no client message id, so the service's uuid
// cannot reach the session file; instead, when pi reports the user message
// it recorded, the adapter records a native event under pi's own identity
// for it, so an import recognizes the prompt (frame, below). a prompt sent
// while a run is active waits for the run to finish (followUp), as one typed
// ahead does.
func (r *runtime) Prompt(_, text string) error {
	r.mu.Lock()
	active := r.turnActive
	r.mu.Unlock()
	command := map[string]any{"type": "prompt", "message": text}
	if active {
		command["streamingBehavior"] = "followUp"
	}
	ctx, cancel := context.WithTimeout(context.Background(), callTimeout)
	defer cancel()
	accepted := &struct{ Disposition string }{}
	if err := r.conn.call(ctx, command, accepted); err != nil {
		var refused *refusal
		if errors.As(err, &refused) {
			// pi answered no: no run started.
			return fmt.Errorf("%w: %w", harness.ErrPromptRefused, err)
		}
		return err
	}
	switch accepted.Disposition {
	case "handled":
		// an extension command consumed it: no run starts, so nothing will
		// end the turn the prompt opened but this.
		r.sink.Event(&model.Event{Kind: model.EventTurnEnd, Status: model.TurnSuccess, Origin: model.OriginManaged, Text: "handled by a pi extension command"})
	case "started":
		r.mu.Lock()
		r.turnActive = true
		r.mu.Unlock()
	}
	return nil
}

// Answer delivers the operator's answer to a pending dialog, or with
// DecisionDismiss pi's own cancel. pi sends no acknowledgment; a dialog it
// already settled (its timeout ran out) is no longer here.
func (r *runtime) Answer(requestID string, answer harness.Answer) error {
	r.mu.Lock()
	d, ok := r.dialogs[requestID]
	if ok && r.stopping && harness.CarriesWork(answer) {
		r.mu.Unlock()
		return harness.ErrNotPending
	}
	if !ok || (!d.deadline.IsZero() && !time.Now().Before(d.deadline)) {
		// past its deadline, pi has settled it and reads no answer; its
		// timer settles the record.
		r.mu.Unlock()
		return harness.ErrNotPending
	}
	response, err := responseFor(d, answer)
	if err != nil {
		r.mu.Unlock()
		return err
	}
	// the dialog goes and the answer is written in one step under the lock,
	// as claude's adapter sends its answer: a settlement handled before this
	// (an aborted run's, or the deadline's) took the dialog already, and one
	// handled after finds nothing left to settle.
	delete(r.dialogs, requestID)
	if d.timer != nil {
		d.timer.Stop()
	}
	err = r.conn.write(response)
	r.mu.Unlock()
	if err != nil {
		return fmt.Errorf("%w: %w", harness.ErrNotPending, err)
	}
	return nil
}

// responseFor is pi's extension_ui_response for an answer (pi 1.5.2, docs/
// rpc-extension-ui.md): a select's choice, a confirm's yes or no, an input's
// or editor's text, or a cancel when the operator dismissed it.
func responseFor(d *dialog, answer harness.Answer) (map[string]any, error) {
	response := map[string]any{"type": "extension_ui_response", "id": d.native}
	if answer.Decision == harness.DecisionDismiss {
		response["cancelled"] = true
		return response, nil
	}
	switch d.method {
	case "select":
		value, ok := answer.Answers[d.title]
		if !ok || !slices.Contains(d.options, value) {
			return nil, fmt.Errorf("%w: '%s' is not one of the choices pi offered", harness.ErrInvalidAnswer, value)
		}
		response["value"] = value
	case "confirm":
		switch answer.Answers[d.title] {
		case confirmYes:
			response["confirmed"] = true
		case confirmNo:
			response["confirmed"] = false
		default:
			return nil, fmt.Errorf("%w: a confirmation is answered %s or %s", harness.ErrInvalidAnswer, confirmYes, confirmNo)
		}
	default:
		value, ok := answer.Answers[formField]
		if !ok {
			return nil, fmt.Errorf("%w: the answer holds no text", harness.ErrInvalidAnswer)
		}
		response["value"] = value
	}
	return response, nil
}

// SetPermissionMode is refused: pi has no permission model. it asks only what
// its extensions ask.
func (r *runtime) SetPermissionMode(string) error {
	return errors.New("pi has no permission modes; it asks only what its extensions ask")
}

// SetModel switches the model and thinking level for later responses, and
// reports what pi then runs with. a level the model does not take is
// clamped by pi (pi 1.5.2, set_thinking_level).
func (r *runtime) SetModel(chosen, effort string) (bool, error) {
	ctx, cancel := context.WithTimeout(context.Background(), callTimeout)
	defer cancel()
	provider, id, ok := strings.Cut(chosen, "/")
	if !ok {
		return false, fmt.Errorf("'%s' is not a pi model, which is provider/id", chosen)
	}
	set := &modelInfo{}
	if err := r.conn.call(ctx, map[string]any{"type": "set_model", "provider": provider, "modelId": id}, set); err != nil {
		return false, err
	}
	if effort != "" {
		if err := r.conn.call(ctx, map[string]any{"type": "set_thinking_level", "level": effort}, nil); err != nil {
			return false, fmt.Errorf("%w: %w", harness.ErrEffortRefused, err)
		}
	}
	state := &state{}
	if err := r.conn.call(ctx, map[string]any{"type": "get_state"}, state); err != nil {
		return false, err
	}
	r.mu.Lock()
	r.model, r.effort = chosen, effort
	r.mu.Unlock()
	r.sink.Event(&model.Event{
		Kind:   model.EventRuntime,
		Status: model.StatusModelChanged,
		Origin: model.OriginManaged,
		Facts:  &model.RuntimeFacts{Model: state.Model.name(), Effort: state.effort()},
	})
	return false, nil
}

// Interrupt aborts the active run. pi answers an abort only once the run is
// idle, and a pending dialog holds the run until it is answered or
// dismissed (R2, capability report), so the abort is sent and not waited
// on; with a dialog pending, the history says what the interrupt waits on.
// mega never answers or dismisses the dialog to let it through.
func (r *runtime) Interrupt() error {
	r.mu.Lock()
	active := r.turnActive
	var waiting []string
	for _, d := range r.dialogs {
		waiting = append(waiting, d.title)
	}
	r.mu.Unlock()
	if !active && len(waiting) == 0 {
		return nil
	}
	if _, _, err := r.conn.send(map[string]any{"type": "abort"}); err != nil {
		return err
	}
	if len(waiting) > 0 {
		slices.Sort(waiting)
		r.sink.Event(&model.Event{
			Kind:   model.EventRuntime,
			Status: model.StatusWaiting,
			Origin: model.OriginManaged,
			Text:   fmt.Sprintf("the interrupt waits on pi's dialog '%s': pi stops once it is answered or dismissed", strings.Join(waiting, "', '")),
		})
	}
	return nil
}

// Close asks pi to stop by closing its input. pi then waits for pending work,
// and a pending dialog holds it open through that and a first SIGTERM
// (pi 1.5.2), so a pi that has not exited is ended: a SIGTERM, a second,
// which pi takes at once, then SIGKILL to its process group, each after a
// bounded wait, sooner when a dialog is pending. the dialog is recorded as
// Stopping fences a stop: from now on no answer to a dialog that would let
// work go on reaches pi, even one accepted before the stop. a dismissal still
// does, which is what lets an interrupt pi holds for a dialog go through.
func (r *runtime) Stopping() {
	r.mu.Lock()
	r.stopping = true
	r.mu.Unlock()
}

// never answered; the command it gated does not run.
func (r *runtime) Close() error {
	r.mu.Lock()
	if r.closing {
		r.mu.Unlock()
		return nil
	}
	r.closing, r.closedAt = true, time.Now()
	r.mu.Unlock()
	err := r.conn.closeInput()
	go r.escalate()
	return err
}

func (r *runtime) escalate() {
	wait := func(d time.Duration) bool {
		select {
		case <-r.conn.done:
			return true
		case <-time.After(d):
			return false
		}
	}
	// the first wait: short with a dialog pending, longer otherwise, rechecked
	// as a dialog may arrive after the close.
	deadline := time.Now().Add(idleGrace)
	for {
		r.mu.Lock()
		pending := len(r.dialogs) > 0
		closedAt := r.closedAt
		r.mu.Unlock()
		if pending && time.Now().After(closedAt.Add(politeGrace)) {
			break
		}
		if time.Now().After(deadline) {
			break
		}
		if wait(250 * time.Millisecond) {
			return
		}
	}
	r.mu.Lock()
	r.escalated = true
	r.mu.Unlock()
	r.conn.signal(syscall.SIGTERM, false)
	if wait(signalGrace) {
		return
	}
	r.conn.signal(syscall.SIGTERM, false)
	if wait(signalGrace) {
		return
	}
	r.conn.signal(syscall.SIGKILL, true)
}

// watch reports the exit once pi is gone: dialogs still pending are recorded
// as never answered, and anything left in pi's process group is named.
func (r *runtime) watch() {
	<-r.conn.done
	// what pi sent before it went is handled first: the exit is the last
	// thing a sink hears.
	close(r.frames)
	<-r.worked
	r.mu.Lock()
	var unanswered []string
	for id, d := range r.dialogs {
		if d.timer != nil {
			d.timer.Stop()
		}
		unanswered = append(unanswered, id)
		delete(r.dialogs, id)
	}
	escalated := r.escalated
	r.mu.Unlock()
	for _, id := range unanswered {
		r.sink.Resolved(id, model.OutcomeUnanswered)
	}
	err := r.conn.err
	if escalated {
		err = fmt.Errorf("pi did not stop when asked, so mega ended its process: %v", err)
	}
	if left := harness.Leftovers(harness.GroupSurvivors(r.conn.cmd.Process.Pid, survivorGrace)); left != nil {
		err = errors.Join(err, left)
	}
	r.sink.Exited(err)
}

// The service records the prompt as it sends it. Keep pi’s echo as native
// bookkeeping and map other live messages immediately in wire order.
func (r *runtime) record(m *message, raw map[string]any) {
	if m.Role == "user" {
		r.sink.Event(&model.Event{Kind: model.EventNative, Origin: model.OriginManaged, Text: "user/recorded by pi"})
		return
	}
	for _, ev := range mapMessage(m, raw, "") {
		ev.Origin = model.OriginManaged
		r.sink.Event(ev)
	}
}

// handle handles one record pi sent besides a response, which arrived at
// at, on work's goroutine.
func (r *runtime) handle(f map[string]any, at time.Time) {
	switch f["type"] {
	case "message_update":
		if update, ok := f["assistantMessageEvent"].(map[string]any); ok && update["type"] == "text_delta" {
			if delta, ok := update["delta"].(string); ok {
				r.sink.Delta(delta)
			}
		}
	case "message_end":
		raw, _ := f["message"].(map[string]any)
		m, ok := bindMessage(raw)
		if !ok {
			return
		}
		if m.Role == "system" {
			return
		}
		if m.Role == "assistant" {
			r.mu.Lock()
			r.lastStop, r.lastError = m.StopReason, m.ErrorMessage
			r.mu.Unlock()
		}
		r.record(m, raw)
	case "agent_start":
		r.mu.Lock()
		r.turnActive = true
		r.mu.Unlock()
	case "agent_settled":
		aborted, _ := f["aborted"].(bool)
		r.mu.Lock()
		stop, failure := r.lastStop, r.lastError
		r.turnActive, r.lastStop, r.lastError = false, "", ""
		// a run that ended aborted or failed is waiting on no dialog: an
		// extension that passed an abort signal to its dialog lets an
		// interrupt cancel it, and pi then no longer reads an answer. such a
		// dialog is settled as aborted, and nothing is sent to pi for it.
		var retired []string
		if aborted || stop == "error" {
			for id, d := range r.dialogs {
				if d.timer != nil {
					d.timer.Stop()
				}
				retired = append(retired, id)
				delete(r.dialogs, id)
			}
		}
		r.mu.Unlock()
		for _, id := range retired {
			r.sink.Resolved(id, model.OutcomeAborted)
		}
		end := &model.Event{Kind: model.EventTurnEnd, Origin: model.OriginManaged, Status: model.TurnSuccess}
		switch {
		case aborted:
			end.Status = model.TurnInterrupted
		case stop == "error":
			end.Status, end.Text = model.TurnError, failure
		}
		r.sink.Event(end)
	case "extension_ui_request":
		r.uiRequest(f, at)
	case "extension_error":
		r.sink.Event(&model.Event{Kind: model.EventNative, Origin: model.OriginManaged, Text: "extension error", Raw: f})
	case "auto_retry_start", "auto_retry_end", "compaction_start", "compaction_end":
		kind, _ := f["type"].(string)
		r.sink.Event(&model.Event{Kind: model.EventNative, Origin: model.OriginManaged, Text: kind, Raw: f})
	}
}

// confirm's two answers, and the one field of an input or editor form.
const (
	confirmYes = "Yes"
	confirmNo  = "No"
	formField  = "value"
)

// uiRequest turns an extension dialog into a question for the operator. pi
// marks no dialog as an approval, so none is one here: every dialog is a
// question, with no session-wide allow, and each offers Dismiss, pi's own
// cancel, for the operator to send. a notification is kept as a native
// event; the other fire-and-forget methods change only pi's terminal.
func (r *runtime) uiRequest(f map[string]any, at time.Time) {
	method, _ := f["method"].(string)
	native, _ := f["id"].(string)
	if method == "notify" {
		message, _ := f["message"].(string)
		r.sink.Event(&model.Event{Kind: model.EventNative, Origin: model.OriginManaged, Text: "pi: " + message, Raw: f})
		return
	}
	if !slices.Contains(dialogMethods, method) || native == "" {
		return
	}
	d := &dialog{native: native, method: method}
	d.title, _ = f["title"].(string)
	if d.title == "" {
		d.title = "pi asks"
	}
	payload := map[string]any{"dismiss": true, "dialog": method}
	switch method {
	case "select":
		var options []any
		for _, o := range asList(f["options"]) {
			if label, ok := o.(string); ok {
				d.options = append(d.options, label)
				options = append(options, map[string]any{"label": label})
			}
		}
		payload["tool"] = "question"
		payload["questions"] = []any{map[string]any{"question": d.title, "options": options}}
	case "confirm":
		message, _ := f["message"].(string)
		question := map[string]any{"question": d.title, "options": []any{map[string]any{"label": confirmYes}, map[string]any{"label": confirmNo}}}
		if message != "" {
			question["header"] = message
		}
		payload["tool"] = "question"
		payload["questions"] = []any{question}
	default:
		field := map[string]any{"name": formField, "type": "string", "required": false}
		if placeholder, _ := f["placeholder"].(string); placeholder != "" {
			field["description"] = placeholder
		}
		if prefill, _ := f["prefill"].(string); prefill != "" {
			field["default"] = prefill
		}
		if method == "editor" {
			field["multiline"] = true
		}
		payload["tool"] = "form"
		payload["message"] = d.title
		payload["fields"] = []any{field}
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	id := r.sink.Request(model.RequestQuestion, payload)
	r.dialogs[id] = d
	if timeout := asMillis(f["timeout"]); timeout > 0 {
		// pi settles a dialog whose timeout runs out on its own, sending
		// nothing (pi 1.5.2): the adapter settles its record the same way,
		// when pi's own time runs out, counted from when the dialog arrived.
		d.deadline = at.Add(timeout)
		d.timer = time.AfterFunc(max(time.Until(d.deadline), 0), func() {
			r.mu.Lock()
			_, still := r.dialogs[id]
			delete(r.dialogs, id)
			r.mu.Unlock()
			if still {
				r.sink.Resolved(id, model.OutcomeExpired)
			}
		})
	}
}

func asList(v any) []any {
	list, _ := v.([]any)
	return list
}

func asMillis(v any) time.Duration {
	switch n := v.(type) {
	case float64:
		return time.Duration(n) * time.Millisecond
	case int64:
		return time.Duration(n) * time.Millisecond
	case int:
		return time.Duration(n) * time.Millisecond
	}
	return 0
}

// modelInfo is the part of pi's Model object the adapter reads.
type modelInfo struct {
	ID        string
	Name      string
	Provider  string
	Reasoning bool
}

// name is a model as mega names it: provider/id.
func (m *modelInfo) name() string {
	if m == nil || m.ID == "" {
		return ""
	}
	return m.Provider + "/" + m.ID
}

// state is the part of get_state the adapter reads (pi 1.5.2).
type state struct {
	SessionID     string     `dd:"sessionId,+nullable"`
	Model         *modelInfo `dd:",+nullable"`
	ThinkingLevel string     `dd:"thinkingLevel"`
	IsStreaming   bool       `dd:"isStreaming"`
}

// effort is the thinking level pi runs with, for a model that takes one; a
// model without reasoning reports "off", which is no effort at all.
func (s *state) effort() string {
	if s.Model == nil || !s.Model.Reasoning {
		return ""
	}
	return s.ThinkingLevel
}
