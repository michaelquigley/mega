package codex

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/michaelquigley/df/dd"
	"github.com/michaelquigley/df/dl"

	"git.hq.quigley.com/research/roundhouse/internal/harness"
	"git.hq.quigley.com/research/roundhouse/internal/model"
)

const (
	// callTimeout bounds a turn request; the turn itself runs as long as it runs.
	callTimeout = time.Minute
	// survivorGrace is how long an exit waits for the app-server's sandboxed
	// commands to go before reporting them still running.
	survivorGrace = 10 * time.Second
)

// methods of the server requests that become permission requests.
const (
	methodCommandApproval     = "item/commandExecution/requestApproval"
	methodFileChangeApproval  = "item/fileChange/requestApproval"
	methodPermissionsApproval = "item/permissions/requestApproval"
	methodUserInput           = "item/tool/requestUserInput"
)

// serverRequest is a request the app-server waits on, by its own JSON-RPC id,
// which restarts at 0 on every connection and never leaves this package.
type serverRequest struct {
	id        any
	method    string
	approval  approvalParams
	userInput []userInputQuestion
}

// question is the adapter's own request for an agentMessage carrying
// questions. codex sends no resolution for it, so its lifecycle is kept here:
// an answer resolves it; interrupt, release, a turn ending failed or
// interrupted, and runtime loss abort it; only a turn that completes keeps it
// pending, and then the answer starts the next turn.
type question struct {
	turn      string
	questions []asyncQuestion
	completed bool // its turn completed with the question unanswered
}

type runtime struct {
	conn *conn
	sink harness.Sink

	mu          sync.Mutex
	threadID    string
	activeTurn  string
	requests    map[string]*serverRequest // service request id -> server request
	byNative    map[string]string         // server request id -> service request id
	questions   map[string]*question      // service request id -> synthetic question
	fileChanges map[string][]fileChange   // item id -> changes, for approval payloads
	completed   map[string]bool           // turns that completed, for answers racing their end
	interrupted map[string]bool           // turns the operator interrupted; no question of theirs may wait
	stops       int                       // interrupts and closes so far; an answer started before one never starts a turn
	started     map[int]proc              // processes seen under the app-server, for the exit check
}

func newRuntime(sink harness.Sink) *runtime {
	return &runtime{
		sink:        sink,
		requests:    make(map[string]*serverRequest),
		byNative:    make(map[string]string),
		questions:   make(map[string]*question),
		fileChanges: make(map[string][]fileChange),
		completed:   make(map[string]bool),
		interrupted: make(map[string]bool),
		started:     make(map[int]proc),
	}
}

// remember notes the processes running under the app-server now, so the exit
// can tell whether any outlived it.
func (r *runtime) remember() {
	found := r.conn.descendants()
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, p := range found {
		r.started[p.pid] = p
	}
}

func (r *runtime) NativeID() string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.threadID
}

func (r *runtime) Prompt(uuid, text string) error {
	r.mu.Lock()
	threadID := r.threadID
	r.mu.Unlock()
	return r.startTurn(threadID, uuid, text)
}

func (r *runtime) startTurn(threadID, uuid, body string) error {
	ctx, cancel := context.WithTimeout(context.Background(), callTimeout)
	defer cancel()
	resp := &turnResponse{}
	if err := r.conn.call(ctx, "turn/start", &turnStartParams{ThreadID: threadID, Input: text(body), ClientUserMessageID: uuid}, resp); err != nil {
		return err
	}
	r.mu.Lock()
	if r.activeTurn == "" && !r.completed[resp.Turn.ID] {
		r.activeTurn = resp.Turn.ID
	}
	r.mu.Unlock()
	return nil
}

// Answer delivers the operator's answer. codex gives no acknowledgment for a
// response to its server request; one it already resolved is ignored by the
// server, and its serverRequest/resolved notice has removed the mapping here,
// so an answer after that reports ErrNotPending.
func (r *runtime) Answer(requestID string, answer harness.Answer) error {
	r.mu.Lock()
	if req, ok := r.requests[requestID]; ok {
		delete(r.requests, requestID)
		delete(r.byNative, nativeKey(req.id))
		r.mu.Unlock()
		return r.conn.respond(req.id, responseFor(req, answer))
	}
	q, ok := r.questions[requestID]
	if !ok {
		r.mu.Unlock()
		return harness.ErrNotPending
	}
	delete(r.questions, requestID)
	threadID, active, stops := r.threadID, r.activeTurn == q.turn && q.turn != "", r.stops
	r.mu.Unlock()

	body := answerText(answer.Answers, q.questions)
	uuid := newID()
	if active {
		ctx, cancel := context.WithTimeout(context.Background(), callTimeout)
		defer cancel()
		err := r.conn.call(ctx, "turn/steer", &turnSteerParams{ThreadID: threadID, ExpectedTurnID: q.turn, Input: text(body), ClientUserMessageID: uuid}, nil)
		if err == nil {
			return nil
		}
		// the turn may have just ended; only a completed one takes the answer
		// as the next turn.
		r.mu.Lock()
		completed := r.completed[q.turn]
		r.mu.Unlock()
		if !completed {
			dl.Debugf("steering an answer into turn '%s': %v", q.turn, err)
			return harness.ErrNotPending
		}
	} else if !q.completed {
		return harness.ErrNotPending
	}
	return r.answerAsTurn(threadID, uuid, body, stops)
}

// answerAsTurn starts the next turn with an answer, unless the operator
// interrupted or released since the answer began: a question never carries
// execution across a stop. a stop that lands while the turn is being started
// interrupts the new turn as soon as it exists.
func (r *runtime) answerAsTurn(threadID, uuid, body string, stops int) error {
	r.mu.Lock()
	if r.stops != stops {
		r.mu.Unlock()
		return harness.ErrNotPending
	}
	r.mu.Unlock()
	ctx, cancel := context.WithTimeout(context.Background(), callTimeout)
	defer cancel()
	resp := &turnResponse{}
	if err := r.conn.call(ctx, "turn/start", &turnStartParams{ThreadID: threadID, Input: text(body), ClientUserMessageID: uuid}, resp); err != nil {
		return err
	}
	r.mu.Lock()
	stopped := r.stops != stops
	if !stopped && r.activeTurn == "" && !r.completed[resp.Turn.ID] {
		r.activeTurn = resp.Turn.ID
	}
	if stopped {
		r.interrupted[resp.Turn.ID] = true
	}
	r.mu.Unlock()
	if !stopped {
		return nil
	}
	if err := r.conn.call(ctx, "turn/interrupt", &turnInterruptParams{ThreadID: threadID, TurnID: resp.Turn.ID}, nil); err != nil {
		dl.Warnf("interrupting a turn an answer started after a stop: %v", err)
	}
	return harness.ErrNotPending
}

func responseFor(req *serverRequest, answer harness.Answer) any {
	allow := answer.Decision == harness.DecisionAllow
	switch req.method {
	case methodPermissionsApproval:
		// grant exactly what was asked, for this turn only, or nothing.
		granted := map[string]any{}
		if allow && req.approval.Permissions != nil {
			granted = req.approval.Permissions
		}
		return &permissionsResult{Permissions: granted, Scope: "turn"}
	case methodUserInput:
		answers := make(map[string]userInputAnswer)
		for _, q := range req.userInput {
			if a, ok := answer.Answers[q.Question]; ok {
				answers[q.ID] = userInputAnswer{Answers: []string{a}}
			}
		}
		return &userInputResult{Answers: answers}
	default:
		if allow {
			return &decisionResult{Decision: "accept"}
		}
		return &decisionResult{Decision: "decline"}
	}
}

// Interrupt stops the active turn. the adapter's own questions are aborted at
// once: none may outlive an interrupt and start new work when answered.
// SetPermissionMode is refused: codex has no permission modes, and its approval
// policy and sandbox come from the operator's own configuration.
func (r *runtime) SetPermissionMode(string) error {
	return errors.New("codex has no permission modes; its approval policy comes from its own configuration")
}

func (r *runtime) Interrupt() error {
	r.remember()
	r.mu.Lock()
	r.stops++
	threadID, turnID := r.threadID, r.activeTurn
	if turnID != "" {
		r.interrupted[turnID] = true
	}
	aborted := r.takeQuestions(func(*question) bool { return true })
	r.mu.Unlock()
	for _, id := range aborted {
		r.sink.Resolved(id, model.OutcomeAborted)
	}
	if turnID == "" {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), callTimeout)
	defer cancel()
	return r.conn.call(ctx, "turn/interrupt", &turnInterruptParams{ThreadID: threadID, TurnID: turnID}, nil)
}

// Close ends input; the app-server exits when it sees the end, and closing is
// itself a clean cancel for a pending approval (capability report).
func (r *runtime) Close() error {
	r.remember()
	r.mu.Lock()
	r.stops++
	r.mu.Unlock()
	return r.conn.closeInput()
}

// takeQuestions removes and returns the synthetic questions match selects.
// callers hold r.mu.
func (r *runtime) takeQuestions(match func(*question) bool) []string {
	var ids []string
	for id, q := range r.questions {
		if match(q) {
			ids = append(ids, id)
			delete(r.questions, id)
		}
	}
	return ids
}

// watch reports the exit once the app-server is gone and every process seen
// under it at an interrupt or a close has gone too, or names what is still
// running when the grace runs out. the capability report saw a sandboxed
// command outlive the app-server after an interrupt.
func (r *runtime) watch() {
	<-r.conn.done
	r.mu.Lock()
	aborted := r.takeQuestions(func(*question) bool { return true })
	var seen []proc
	for _, p := range r.started {
		seen = append(seen, p)
	}
	r.mu.Unlock()
	for _, id := range aborted {
		r.sink.Resolved(id, model.OutcomeAborted)
	}
	err := r.conn.err
	if pids := survivors(seen, survivorGrace); len(pids) > 0 {
		err = errors.Join(err, fmt.Errorf("%w: pids %v", harness.ErrLeftRunning, pids))
	}
	r.sink.Exited(err)
}

func (r *runtime) notify(method string, params map[string]any) {
	switch method {
	case "turn/started":
		n := &turnNotification{}
		if bind(n, params) {
			r.mu.Lock()
			r.activeTurn = n.Turn.ID
			r.mu.Unlock()
		}
	case "turn/completed":
		r.turnCompleted(params)
	case "item/started":
		n := &itemNotification{}
		if bind(n, params) && n.Item != nil {
			if it, err := bindItem(n.Item); err == nil && it.Type == "fileChange" {
				r.mu.Lock()
				r.fileChanges[it.ID] = it.Changes
				r.mu.Unlock()
			}
		}
	case "item/completed":
		r.itemCompleted(params)
	case "item/agentMessage/delta":
		n := &deltaNotification{}
		if bind(n, params) && n.Delta != "" {
			r.sink.Delta(n.Delta)
		}
	case "serverRequest/resolved":
		n := &resolvedNotification{}
		if !bind(n, params) {
			return
		}
		r.mu.Lock()
		id, ok := r.byNative[nativeKey(n.RequestID)]
		delete(r.byNative, nativeKey(n.RequestID))
		delete(r.requests, id)
		r.mu.Unlock()
		if ok {
			r.sink.Resolved(id, model.OutcomeAborted)
		}
	case "error":
		r.sink.Event(&model.Event{Kind: model.EventNative, Origin: model.OriginManaged, Text: "error", Raw: params})
	default:
		// transport chatter: status flags, token usage, reasoning and output
		// deltas, MCP startup. the items carry the conversation.
	}
}

func (r *runtime) turnCompleted(params map[string]any) {
	n := &turnNotification{}
	if !bind(n, params) {
		return
	}
	r.mu.Lock()
	// a turn the operator interrupted keeps no question, even if it reports
	// completing because it finished before the interrupt landed.
	completed := n.Turn.Status == "completed" && !r.interrupted[n.Turn.ID]
	if r.activeTurn == n.Turn.ID {
		r.activeTurn = ""
	}
	if completed {
		r.completed[n.Turn.ID] = true
		for _, q := range r.questions {
			if q.turn == n.Turn.ID {
				q.completed = true
			}
		}
	}
	aborted := r.takeQuestions(func(q *question) bool { return q.turn == n.Turn.ID && !completed })
	r.mu.Unlock()
	for _, id := range aborted {
		r.sink.Resolved(id, model.OutcomeAborted)
	}
	r.sink.Event(&model.Event{Kind: model.EventTurnEnd, Origin: model.OriginManaged, Status: turnStatus(n.Turn.Status), Text: n.Turn.Status, Raw: params})
}

func (r *runtime) itemCompleted(params map[string]any) {
	n := &itemNotification{}
	if !bind(n, params) || n.Item == nil {
		return
	}
	evs, err := mapItem(n.Item)
	if err != nil {
		dl.Warnf("codex thread '%s': %v", r.NativeID(), err)
		return
	}
	for _, ev := range evs {
		ev.Origin = model.OriginManaged
		r.sink.Event(ev)
	}
	it, err := bindItem(n.Item)
	if err != nil || it.Type != "agentMessage" || len(it.Questions) == 0 {
		return
	}
	// hold r.mu across the sink call, so an answer cannot arrive before the
	// question is known here.
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.interrupted[n.TurnID] {
		// the operator interrupted this turn; a question it still produced must
		// not wait for an answer that would start new work.
		return
	}
	id := r.sink.Request(model.RequestQuestion, questionPayload(it.Questions))
	r.questions[id] = &question{turn: n.TurnID, questions: it.Questions, completed: r.completed[n.TurnID]}
}

func (r *runtime) request(id any, method string, params map[string]any) {
	req := &serverRequest{id: id, method: method}
	kind := model.RequestPermission
	var payload map[string]any
	switch method {
	case methodCommandApproval, methodFileChangeApproval, methodPermissionsApproval:
		if !bind(&req.approval, params) {
			return
		}
		payload = r.approvalPayload(method, &req.approval)
	case methodUserInput:
		p := &userInputParams{}
		if !bind(p, params) {
			return
		}
		req.userInput = p.Questions
		kind = model.RequestQuestion
		payload = userInputPayload(p.Questions)
	default:
		// not something roundhouse can put to the operator. it is not answered
		// either way on the operator's behalf: the turn waits, and the record says
		// so, until the operator interrupts it.
		r.sink.Event(&model.Event{Kind: model.EventNative, Origin: model.OriginManaged, Text: fmt.Sprintf("codex asked '%s', which roundhouse cannot answer; interrupt the turn to go on", method), Raw: params})
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	svc := r.sink.Request(kind, payload)
	r.requests[svc] = req
	r.byNative[nativeKey(id)] = svc
}

func (r *runtime) approvalPayload(method string, p *approvalParams) map[string]any {
	payload := map[string]any{}
	if p.Reason != "" {
		payload["description"] = p.Reason
	}
	switch method {
	case methodCommandApproval:
		payload["tool"] = "shell"
		payload["input"] = map[string]any{"command": p.Command, "cwd": p.Cwd}
	case methodFileChangeApproval:
		r.mu.Lock()
		changes := r.fileChanges[p.ItemID]
		r.mu.Unlock()
		var paths []any
		for _, change := range changes {
			paths = append(paths, change.Path)
		}
		payload["tool"] = "edit"
		payload["input"] = map[string]any{"paths": paths}
		if p.GrantRoot != "" {
			payload["blocked_path"] = p.GrantRoot
		}
	case methodPermissionsApproval:
		payload["tool"] = "permissions"
		payload["input"] = p.Permissions
	}
	return payload
}

func userInputPayload(questions []userInputQuestion) map[string]any {
	var list []any
	for _, q := range questions {
		var options []any
		for _, o := range q.Options {
			options = append(options, map[string]any{"label": o.Label, "description": o.Description})
		}
		list = append(list, map[string]any{"question": q.Question, "header": q.Header, "options": options})
	}
	return map[string]any{"tool": "question", "questions": list}
}

func bind(target any, params map[string]any) bool {
	if params == nil {
		return false
	}
	if err := dd.Bind(target, params); err != nil {
		dl.Warnf("reading codex frame: %v", err)
		return false
	}
	return true
}

// nativeKey normalizes a JSON-RPC id, which arrives as a number or a string.
func nativeKey(id any) string {
	if f, ok := id.(float64); ok && f == float64(int64(f)) {
		return fmt.Sprintf("%d", int64(f))
	}
	return fmt.Sprint(id)
}
