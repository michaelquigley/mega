package claude

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/michaelquigley/df/dd"
	"github.com/michaelquigley/df/dl"

	"github.com/michaelquigley/mega/internal/harness"
	"github.com/michaelquigley/mega/internal/model"
)

// shimFrame is one line from the shim. the shim's protocol is documented in
// shim/claude/shim.mjs.
type shimFrame struct {
	Type           string
	ID             string
	Kind           string
	Outcome        string
	Message        string
	Code           int
	Msg            map[string]any `dd:",+opaque,+nullable"`
	Tool           string
	Input          any    `dd:",+opaque,+nullable"`
	ToolUseID      string `dd:"toolUseId"`
	Title          string `dd:",+nullable"`
	DisplayName    string `dd:"displayName,+nullable"`
	Description    string `dd:",+nullable"`
	DecisionReason string `dd:"decisionReason,+nullable"`
	BlockedPath    string `dd:"blockedPath,+nullable"`
	Suggestions    any    `dd:",+opaque,+nullable"`
	Mode           string `dd:",+nullable"`
	Error          string `dd:",+nullable"`
	Version        string `dd:",+nullable"`
	Model          string `dd:",+nullable"`
}

// command frames sent to the shim.
type promptFrame struct {
	Type string
	UUID string
	Text string
}

type answerFrame struct {
	Type     string
	ID       string
	Decision string            `dd:",+omitempty"`
	Message  string            `dd:",+omitempty"`
	Answers  map[string]string `dd:",+omitempty"`
}

type controlFrame struct {
	Type string
}

// modeSwitch is a mode switch waiting for the shim's reply. only the reply
// handler removes it: it applies a confirmed mode and reports it as a mode
// change before waking the waiter, so every mode fact reaches the sink in the
// order claude reported it, and whatever the reader handles next already sees
// it. done is buffered, so a waiter that gave up never blocks the reader.
type modeSwitch struct {
	kind string // "" for a permission mode, else "model" or "effort"
	mode string
	done chan error
}

type settingFrame struct {
	Type   string
	ID     string
	Model  string `dd:",+omitempty"`
	Effort string `dd:",+omitempty"`
}

type modeFrame struct {
	Type string
	ID   string
	Mode string
}

// modeTimeout bounds the wait for the shim to confirm a mode switch, which the
// sdk answers at once (capability report, 2.1.318).
var modeTimeout = 30 * time.Second

// runtime is one shim process bound to one claude session.
type runtime struct {
	nativeID string
	cmd      *exec.Cmd
	stdin    io.WriteCloser
	sink     harness.Sink

	mu        sync.Mutex
	requests  map[string]string      // service request id -> sdk requestId
	byNative  map[string]string      // sdk requestId -> service request id
	answers   map[string]chan error  // sdk requestId -> the answer waiting for the shim's verdict
	modes     map[string]*modeSwitch // mode switch id -> the switch waiting for the shim's reply
	nextMode  int
	inherited bool // no permission mode was passed; the operator's settings chose it
	// launchMode is the mode the launch asked for: the one passed, or the one
	// the operator's settings name.
	launchMode string
	reqModel   string // the model last passed or confirmed; empty if none was chosen
	reqEffort  string // the effort last passed or confirmed; empty if none was chosen
	liveModel  string // the model as system/init last reported it
	// backgroundSeen is when each background task still running was first
	// seen, by task id.
	backgroundSeen map[string]time.Time
	mode           string // the permission mode as last reported or confirmed
	// modeRequested: mode is only what the launch asked for, as an idle
	// launch reports it, until claude reports or confirms one.
	modeRequested bool
	started       bool // the first init, or an idle launch's ready, has been reported
	// unverified: an idle launch, whose ready reports no session, has not yet
	// seen claude report one; the first is checked against nativeID.
	unverified  bool
	interrupted bool   // an interrupt was sent during the current turn
	gone        bool   // the shim's output has ended
	failure     string // the shim's last error frame
}

// errExited fails an answer whose runtime ended before the shim confirmed it.
var errExited = errors.New("the claude runtime exited before the answer was confirmed")

// start runs the shim. each failure here comes before or while starting the
// shim process, so no session can exist and each is marked not started.
func start(cmd *exec.Cmd, nativeID string, sink harness.Sink, inherited bool, launchMode string, idle bool) (*runtime, error) {
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("%w: %w", harness.ErrNotStarted, err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("%w: %w", harness.ErrNotStarted, err)
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return nil, fmt.Errorf("%w: %w", harness.ErrNotStarted, err)
	}
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("%w: starting claude shim: %w", harness.ErrNotStarted, err)
	}
	r := &runtime{
		nativeID:   nativeID,
		cmd:        cmd,
		stdin:      stdin,
		sink:       sink,
		requests:   make(map[string]string),
		byNative:   make(map[string]string),
		answers:    make(map[string]chan error),
		modes:      make(map[string]*modeSwitch),
		inherited:  inherited,
		launchMode: launchMode,
		// an idle launch has no first init to carry the session to the
		// service before it shows the runtime started; the first session
		// claude reports, from its very first frame, is checked here instead.
		unverified: idle,
	}
	go logStderr(nativeID, stderr)
	go r.read(stdout)
	if idle {
		time.AfterFunc(idleReadyTimeout, r.readyOverdue)
	}
	return r, nil
}

// idleReadyTimeout bounds how long a start with no prompt waits for claude to
// answer: past it, the runtime is stopped, so the conversation is left with
// no runtime and can be started again rather than starting for ever. the
// shim's own wait on initialize is 30 seconds, inside this one.
var idleReadyTimeout = time.Minute

// readyOverdue stops an idle launch that has not reported started in time:
// the reason is kept for the exit, and the shim's process group (the shim,
// claude, and anything claude started) is killed, since claude never answered
// and nothing of the session was written. its exit is reported as for any
// runtime, and the service records the error.
func (r *runtime) readyOverdue() {
	r.mu.Lock()
	if r.started || r.gone || r.cmd == nil || r.cmd.Process == nil {
		r.mu.Unlock()
		return
	}
	reason := fmt.Sprintf("claude did not answer within %s of a start with no prompt, so the runtime was stopped", idleReadyTimeout)
	if r.failure != "" {
		reason = r.failure + "; " + reason
	}
	r.failure = reason
	pid := r.cmd.Process.Pid
	r.mu.Unlock()
	dl.Warnf("claude shim for '%s': %s", r.nativeID, reason)
	if err := syscall.Kill(-pid, syscall.SIGKILL); err != nil {
		dl.Warnf("stopping claude shim for '%s': %v", r.nativeID, err)
	}
}

func (r *runtime) NativeID() string {
	return r.nativeID
}

func (r *runtime) Prompt(uuid, text string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.interrupted = false
	return r.send(&promptFrame{Type: "prompt", UUID: uuid, Text: text})
}

// Answer returns once the shim says whether the request was still waiting. a
// request the sdk aborted in the meantime is rejected by the shim, and reports
// ErrNotPending rather than success.
func (r *runtime) Answer(requestID string, answer harness.Answer) error {
	r.mu.Lock()
	native, ok := r.requests[requestID]
	if !ok {
		r.mu.Unlock()
		return harness.ErrNotPending
	}
	if r.gone {
		r.mu.Unlock()
		return errExited
	}
	delete(r.requests, requestID)
	delete(r.byNative, native)
	verdict := make(chan error, 1)
	r.answers[native] = verdict
	if err := r.send(&answerFrame{Type: "answer", ID: native, Decision: answer.Decision, Message: answer.Message, Answers: answer.Answers}); err != nil {
		delete(r.answers, native)
		r.mu.Unlock()
		return err
	}
	r.mu.Unlock()
	return <-verdict
}

// verdict settles an answer waiting on the shim.
func (r *runtime) verdict(native string, err error) {
	r.mu.Lock()
	waiting, ok := r.answers[native]
	delete(r.answers, native)
	r.mu.Unlock()
	if ok {
		waiting <- err
	} else {
		dl.Warnf("claude shim for '%s' confirmed an answer nobody was waiting for", r.nativeID)
	}
}

func (r *runtime) Interrupt() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.interrupted = true
	return r.send(&controlFrame{Type: "interrupt"})
}

// modeChanged is the event for a permission mode claude confirmed or reported.
func modeChanged(mode, text string) *model.Event {
	return &model.Event{
		Kind:   model.EventRuntime,
		Status: model.StatusModeChanged,
		Origin: model.OriginManaged,
		Text:   text,
		Facts:  &model.RuntimeFacts{PermissionMode: mode},
	}
}

// SetPermissionMode asks the shim to switch the session's mode and returns once
// the sdk has confirmed or refused it. the switch applies from the next tool
// call; a request already pending stays pending.
func (r *runtime) SetPermissionMode(mode string) error {
	r.mu.Lock()
	if r.gone {
		r.mu.Unlock()
		return errExited
	}
	r.nextMode++
	id := fmt.Sprintf("mode-%d", r.nextMode)
	done := make(chan error, 1)
	r.modes[id] = &modeSwitch{mode: mode, done: done}
	if err := r.send(&modeFrame{Type: "set_permission_mode", ID: id, Mode: mode}); err != nil {
		delete(r.modes, id)
		r.mu.Unlock()
		return err
	}
	r.mu.Unlock()
	// the switch stays registered until its reply is read, even after the
	// timeout, so a late confirmation is still applied and reported.
	select {
	case err := <-done:
		return err
	case <-time.After(modeTimeout):
		return fmt.Errorf("%w: claude did not confirm the switch to '%s' within %s", harness.ErrModeUnconfirmed, mode, modeTimeout)
	}
}

// SetModel asks claude for the model and effort the operator chose, sending
// only what differs from what was last passed or confirmed, and returns once
// claude has confirmed each. the change applies from the next response. the
// live model is not written here: the next system/init reports it. claude
// never reports effort, so a confirmed effort is only what was requested.
func (r *runtime) SetModel(model, effort string) (bool, error) {
	r.mu.Lock()
	current, currentEffort := r.reqModel, r.reqEffort
	r.mu.Unlock()
	modelSent := model != "" && model != current
	if modelSent {
		if err := r.setSetting("model", model); err != nil {
			return false, err
		}
	}
	if effort != "" && effort != currentEffort {
		if err := r.setSetting("effort", effort); err != nil {
			if modelSent && !errors.Is(err, harness.ErrSettingUnconfirmed) {
				return false, fmt.Errorf("%w: %w", harness.ErrEffortRefused, err)
			}
			return false, err
		}
	}
	return false, nil
}

// setSetting sends set_model or set_effort and waits for the shim's reply.
func (r *runtime) setSetting(kind, value string) error {
	r.mu.Lock()
	if r.gone {
		r.mu.Unlock()
		return errExited
	}
	r.nextMode++
	id := fmt.Sprintf("%s-%d", kind, r.nextMode)
	done := make(chan error, 1)
	r.modes[id] = &modeSwitch{kind: kind, mode: value, done: done}
	frame := &settingFrame{Type: "set_" + kind, ID: id}
	if kind == "model" {
		frame.Model = value
	} else {
		frame.Effort = value
	}
	if err := r.send(frame); err != nil {
		delete(r.modes, id)
		r.mu.Unlock()
		return err
	}
	r.mu.Unlock()
	select {
	case err := <-done:
		return err
	case <-time.After(modeTimeout):
		return fmt.Errorf("%w: claude did not confirm %s '%s' within %s", harness.ErrSettingUnconfirmed, kind, value, modeTimeout)
	}
}

func (r *runtime) Close() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.send(&controlFrame{Type: "close"})
}

// send writes one frame. callers hold r.mu.
func (r *runtime) send(frame any) error {
	line, err := dd.UnbindJSONL(frame)
	if err != nil {
		return err
	}
	if _, err := r.stdin.Write(line); err != nil {
		return fmt.Errorf("writing to claude shim: %w", err)
	}
	return nil
}

// read drains the shim's stdout until it exits, then reports the exit.
func (r *runtime) read(stdout io.Reader) {
	reader := bufio.NewReader(stdout)
	for {
		line, err := reader.ReadBytes('\n')
		if len(strings.TrimSpace(string(line))) > 0 {
			r.handle(line)
		}
		if err != nil {
			if !errors.Is(err, io.EOF) {
				dl.Warnf("reading claude shim for '%s': %v", r.nativeID, err)
			}
			break
		}
	}
	r.mu.Lock()
	r.gone = true
	for native, waiting := range r.answers {
		waiting <- errExited
		delete(r.answers, native)
	}
	for id, waiting := range r.modes {
		waiting.done <- errExited
		delete(r.modes, id)
	}
	r.mu.Unlock()
	err := r.cmd.Wait()
	r.mu.Lock()
	if err == nil && r.failure != "" {
		err = errors.New(r.failure)
	} else if err != nil && r.failure != "" {
		err = fmt.Errorf("%w: %s", err, r.failure)
	}
	r.mu.Unlock()
	r.sink.Exited(err)
}

func (r *runtime) handle(line []byte) {
	frame := &shimFrame{}
	if err := dd.BindJSON(frame, line); err != nil {
		dl.Warnf("unreadable frame from claude shim for '%s': %v", r.nativeID, err)
		return
	}
	switch frame.Type {
	case "event":
		r.event(frame.Msg)
	case "request":
		r.request(frame)
	case "request_resolved":
		r.mu.Lock()
		id, ok := r.byNative[frame.ID]
		delete(r.byNative, frame.ID)
		delete(r.requests, id)
		r.mu.Unlock()
		if ok {
			r.sink.Resolved(id, frame.Outcome)
		}
	case "answer_accepted":
		r.verdict(frame.ID, nil)
	case "answer_rejected":
		r.verdict(frame.ID, harness.ErrNotPending)
	case "error":
		dl.Warnf("claude shim for '%s': %s", r.nativeID, frame.Message)
		r.mu.Lock()
		r.failure = frame.Message
		r.mu.Unlock()
	case "model_set", "effort_set":
		r.mu.Lock()
		waiting, ok := r.modes[frame.ID]
		delete(r.modes, frame.ID)
		if ok && frame.Error == "" {
			if waiting.kind == "model" {
				r.reqModel = waiting.mode
			} else {
				r.reqEffort = waiting.mode
			}
		}
		r.mu.Unlock()
		if !ok {
			dl.Warnf("claude shim for '%s' confirmed a '%s' change it was never asked for", r.nativeID, frame.Type)
			return
		}
		if frame.Error != "" {
			waiting.done <- fmt.Errorf("claude refused %s '%s': %s", waiting.kind, waiting.mode, frame.Error)
			return
		}
		waiting.done <- nil
	case "permission_mode_set":
		r.mu.Lock()
		waiting, ok := r.modes[frame.ID]
		delete(r.modes, frame.ID)
		changed := false
		if ok && frame.Error == "" {
			// a confirmed switch is a report, even to the mode requested.
			changed = r.mode != waiting.mode || r.modeRequested
			r.mode = waiting.mode
			r.modeRequested = false
		}
		r.mu.Unlock()
		if !ok {
			dl.Warnf("claude shim for '%s' confirmed a mode switch it was never asked for", r.nativeID)
			return
		}
		if frame.Error != "" {
			waiting.done <- fmt.Errorf("claude refused permission mode '%s': %s", waiting.mode, frame.Error)
			return
		}
		// reported before the waiter wakes, whether or not it is still
		// waiting, so the mode reaches the sink in claude's order.
		if changed {
			r.sink.Event(modeChanged(waiting.mode, "confirmed by claude; applies from the next tool call"))
		}
		waiting.done <- nil
	case "ready":
		r.ready(frame)
	case "interrupted", "exit":
	default:
		dl.Debugf("unknown frame '%s' from claude shim for '%s'", frame.Type, r.nativeID)
	}
}

// ready is an idle launch's report that the session is up: claude's version
// and model, answered without a turn. it shows the runtime started, with the
// mode the launch asked for marked as requested, since claude reports its mode
// only at a turn's start. a first init that came sooner has already done so.
func (r *runtime) ready(frame *shimFrame) {
	r.mu.Lock()
	if r.started {
		r.mu.Unlock()
		return
	}
	r.started = true
	r.mode = r.launchMode
	r.modeRequested = r.launchMode != ""
	if frame.Model != "" {
		r.liveModel = frame.Model
	}
	// no session: claude has reported none yet. the first message that
	// carries one is checked against the session resumed.
	facts := &model.RuntimeFacts{
		Model:          frame.Model,
		Version:        frame.Version,
		PermissionMode: r.launchMode,
		ModeInherited:  r.inherited,
		ModeRequested:  r.launchMode != "",
	}
	r.mu.Unlock()
	r.sink.Event(&model.Event{Kind: model.EventRuntime, Status: model.StatusStarted, Origin: model.OriginManaged, Text: "started with no prompt", Facts: facts})
}

// verifySession checks the first session claude reports in an idle launch,
// before or after its ready, against the session resumed. a different one is reported as
// the session the runtime started on, so the service's guard records the
// error and stops it, as it does for a first init that disagrees.
func (r *runtime) verifySession(reported string) {
	r.mu.Lock()
	check := r.unverified
	r.unverified = false
	r.mu.Unlock()
	if check && reported != r.nativeID {
		r.sink.Event(&model.Event{Kind: model.EventRuntime, Status: model.StatusStarted, Origin: model.OriginManaged, Text: "claude reported another session", Facts: &model.RuntimeFacts{Session: reported}})
	}
}

func (r *runtime) event(raw map[string]any) {
	if raw == nil {
		return
	}
	msg, err := bindMessage(raw)
	if err != nil {
		dl.Warnf("claude shim for '%s': %v", r.nativeID, err)
		return
	}
	if msg.SessionID != "" {
		r.verifySession(msg.SessionID)
	}
	if msg.Type == "system" && msg.Subtype == "background_tasks_changed" {
		// the whole current list of background work; the native event is
		// still recorded below, as before.
		r.backgroundChanged(raw)
	}
	if msg.Type == "stream_event" {
		if text := delta(msg); text != "" {
			r.sink.Delta(text)
		}
		return
	}
	if msg.Type == "system" && msg.Subtype == "init" {
		r.mu.Lock()
		first := !r.started
		r.started = true
		previous := r.mode
		requested := r.modeRequested
		if msg.PermissionMode != "" {
			r.mode = msg.PermissionMode
			r.modeRequested = false
		}
		previousModel := r.liveModel
		if msg.Model != "" {
			r.liveModel = msg.Model
		}
		inherited := r.inherited
		r.mu.Unlock()
		if !first && msg.Model != "" && msg.Model != previousModel {
			// the model as claude reports it at this turn's start, after a
			// switch or any change of its own: the one writer of the live model.
			r.sink.Event(&model.Event{Kind: model.EventRuntime, Status: model.StatusModelChanged, Origin: model.OriginManaged, Text: "reported by claude", Facts: &model.RuntimeFacts{Model: msg.Model}})
		}
		switch {
		case !first && msg.PermissionMode != "" && requested:
			// the first turn after an idle launch: claude's own report replaces
			// the mode the launch only requested, the same one or not.
			same := msg.PermissionMode == previous
			r.sink.Event(&model.Event{
				Kind:   model.EventRuntime,
				Status: model.StatusModeChanged,
				Origin: model.OriginManaged,
				Text:   "reported by claude at the first turn",
				Facts:  &model.RuntimeFacts{PermissionMode: msg.PermissionMode, ModeInherited: same && inherited, ModeConfirmed: same},
			})
		case !first && msg.PermissionMode != "" && msg.PermissionMode != previous:
			// claude re-reports init at the start of every turn. a mode that
			// differs from the last one known changed inside the session, as
			// when plan mode is left.
			r.sink.Event(modeChanged(msg.PermissionMode, "reported by claude"))
		}
		if first {
			r.sink.Event(&model.Event{
				Kind:   model.EventRuntime,
				Status: model.StatusStarted,
				Origin: model.OriginManaged,
				Facts: &model.RuntimeFacts{
					Session:        msg.SessionID,
					Model:          msg.Model,
					Version:        msg.ClaudeCodeVersion,
					PermissionMode: msg.PermissionMode,
					ModeInherited:  inherited,
					Cwd:            msg.Cwd,
				},
				Raw: raw,
			})
			return
		}
	}
	evs, err := mapMessage(raw)
	if err != nil {
		dl.Warnf("claude shim for '%s': %v", r.nativeID, err)
		return
	}
	for _, ev := range evs {
		ev.Origin = model.OriginManaged
		if ev.Kind == model.EventTurnEnd {
			r.mu.Lock()
			if r.interrupted {
				ev.Status = model.TurnInterrupted
			}
			r.interrupted = false
			r.mu.Unlock()
		}
		r.sink.Event(ev)
	}
}

// request asks the service for an id while holding r.mu, so an answer cannot
// arrive before the mapping to the sdk's requestId exists.
func (r *runtime) request(frame *shimFrame) {
	payload := map[string]any{"tool": frame.Tool}
	if frame.Kind == model.RequestQuestion {
		if input, ok := frame.Input.(map[string]any); ok {
			payload["questions"] = input["questions"]
		}
	} else {
		payload["input"] = frame.Input
		for key, value := range map[string]string{
			"title": frame.Title, "display_name": frame.DisplayName, "description": frame.Description,
			"decision_reason": frame.DecisionReason, "blocked_path": frame.BlockedPath,
		} {
			if value != "" {
				payload[key] = value
			}
		}
	}
	kind := model.RequestPermission
	if frame.Kind == model.RequestQuestion {
		kind = model.RequestQuestion
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	// the mode the request was raised in: in auto mode, a request is one the
	// classifier and the checkout's rules did not settle on their own.
	if r.mode != "" {
		payload["permission_mode"] = r.mode
	}
	id := r.sink.Request(kind, payload)
	r.requests[id] = frame.ID
	r.byNative[frame.ID] = id
}

func logStderr(nativeID string, stderr io.Reader) {
	scanner := bufio.NewScanner(stderr)
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)
	for scanner.Scan() {
		if line := strings.TrimSpace(scanner.Text()); line != "" {
			dl.Debugf("claude '%s': %s", nativeID, line)
		}
	}
}

// backgroundChanged reports a background_tasks_changed snapshot to the sink as
// the whole list. each snapshot carries every task still running (claude code
// 2.1.318: one on every change, down to an empty list; capability report,
// 2026-11-10). tasks carry no start time, so a task's start is when the
// adapter first saw it, kept across snapshots.
func (r *runtime) backgroundChanged(raw map[string]any) {
	tasks, _ := raw["tasks"].([]any)
	now := time.Now().UTC()
	r.mu.Lock()
	seen := make(map[string]time.Time, len(tasks))
	jobs := make([]model.BackgroundJob, 0, len(tasks))
	for _, entry := range tasks {
		task, ok := entry.(map[string]any)
		if !ok {
			continue
		}
		id, _ := task["task_id"].(string)
		if id == "" {
			continue
		}
		description, _ := task["description"].(string)
		kind, _ := task["task_type"].(string)
		started, ok := r.backgroundSeen[id]
		if !ok {
			started = now
		}
		seen[id] = started
		jobs = append(jobs, model.BackgroundJob{ID: id, Description: description, Kind: kind, StartedAt: started})
	}
	r.backgroundSeen = seen
	r.mu.Unlock()
	r.sink.Background(jobs)
}
