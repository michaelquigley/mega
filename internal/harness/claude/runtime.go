package claude

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"strings"
	"sync"

	"github.com/michaelquigley/df/dd"
	"github.com/michaelquigley/df/dl"

	"git.hq.quigley.com/research/roundhouse/internal/harness"
	"git.hq.quigley.com/research/roundhouse/internal/model"
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

// runtime is one shim process bound to one claude session.
type runtime struct {
	nativeID string
	cmd      *exec.Cmd
	stdin    io.WriteCloser
	sink     harness.Sink

	mu          sync.Mutex
	requests    map[string]string     // service request id -> sdk requestId
	byNative    map[string]string     // sdk requestId -> service request id
	answers     map[string]chan error // sdk requestId -> the answer waiting for the shim's verdict
	started     bool                  // the first init has been reported
	interrupted bool                  // an interrupt was sent during the current turn
	gone        bool                  // the shim's output has ended
	failure     string                // the shim's last error frame
}

// errExited fails an answer whose runtime ended before the shim confirmed it.
var errExited = errors.New("the claude runtime exited before the answer was confirmed")

func start(cmd *exec.Cmd, nativeID string, sink harness.Sink) (*runtime, error) {
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("starting claude shim: %w", err)
	}
	r := &runtime{
		nativeID: nativeID,
		cmd:      cmd,
		stdin:    stdin,
		sink:     sink,
		requests: make(map[string]string),
		byNative: make(map[string]string),
		answers:  make(map[string]chan error),
	}
	go logStderr(nativeID, stderr)
	go r.read(stdout)
	return r, nil
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
	case "interrupted", "exit":
	default:
		dl.Debugf("unknown frame '%s' from claude shim for '%s'", frame.Type, r.nativeID)
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
		r.mu.Unlock()
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
