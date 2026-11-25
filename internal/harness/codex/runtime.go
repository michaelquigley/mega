package codex

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/michaelquigley/df/dd"
	"github.com/michaelquigley/df/dl"

	"github.com/michaelquigley/mega/internal/harness"
	"github.com/michaelquigley/mega/internal/model"
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
	id          any
	method      string
	approval    approvalParams
	userInput   []userInputQuestion
	elicitation *elicitation
}

// question is the adapter's own request for an agentMessage carrying
// questions. codex sends no resolution for it, so its life is kept here: an
// answer steered into its turn resolves it; its turn ending, an interrupt, a
// release and runtime loss abort it. an answer never starts a turn.
type question struct {
	turn      string
	questions []asyncQuestion
}

type runtime struct {
	conn *conn
	sink harness.Sink

	mu       sync.Mutex
	threadID string
	// expected is the thread a resume asked for, known before codex confirms
	// it: until then only frames of that thread are taken.
	expected    string
	activeTurn  string
	requests    map[string]*serverRequest  // service request id -> server request
	byNative    map[string]string          // server request id -> service request id
	questions   map[string]*question       // service request id -> synthetic question
	fileChanges map[string][]fileChange    // item id -> changes, for approval payloads
	ended       map[string]bool            // turns codex reported ended, in any status; written only by the reader
	interrupted map[string]bool            // turns the operator interrupted; no question of theirs may wait
	started     map[int]proc               // processes seen under the app-server, for the exit check
	unlisted    error                      // why the processes under it could not be listed, if they could not
	mcpCalls    []mcpCall                  // MCP tool calls in flight, oldest first, to name the tool an approval is for
	commands    map[string]*runningCommand // shell commands started and not yet completed, by item id
	model       string                     // the conversation's model and effort, sent on every turn/start
	effort      string
	liveModel   string // the model and effort as codex last reported them
	liveEffort  string
	// the MCP servers: a list running, and another wanted after it.
	mcpBusy  bool
	mcpAgain bool
	// mcpReport orders the MCP reports against the exit: once mcpExited is
	// set, none is made.
	mcpReport sync.Mutex
	mcpExited bool
	stopping  bool // a fenced stop: no answer that would let work go on is delivered (harness.CarriesWork)
	// aborting is held by abortQuestions throughout. steering counts written
	// steers not yet settled, each in the hold of r.mu that writes it, which
	// watch takes before it waits for them.
	aborting     sync.Mutex
	steering     sync.WaitGroup
	steerTimeout time.Duration
}

func newRuntime(sink harness.Sink) *runtime {
	return &runtime{
		sink:        sink,
		requests:    make(map[string]*serverRequest),
		byNative:    make(map[string]string),
		questions:   make(map[string]*question),
		fileChanges: make(map[string][]fileChange),
		commands:    make(map[string]*runningCommand),
		ended:       make(map[string]bool),
		interrupted: make(map[string]bool),
		started:     make(map[int]proc),
		// steerTimeout is callTimeout but in tests.
		steerTimeout: callTimeout,
	}
}

// remember notes the processes running under the app-server now, so the exit
// can tell whether any outlived it. a listing that cannot be read is kept,
// so the exit says it could not check rather than that nothing survived.
func (r *runtime) remember() {
	found, err := r.conn.descendants()
	r.mu.Lock()
	defer r.mu.Unlock()
	if err != nil {
		r.unlisted = err
	}
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
	if err := r.conn.call(ctx, "turn/start", r.turnStart(threadID, uuid, body), resp); err != nil {
		var refused *replyError
		if errors.As(err, &refused) {
			// codex answered with an error: no turn started.
			return fmt.Errorf("%w: %w", harness.ErrPromptRefused, err)
		}
		return err
	}
	r.mu.Lock()
	if r.activeTurn == "" && !r.ended[resp.Turn.ID] {
		r.namedLocked(resp.Turn.ID)
	}
	r.mu.Unlock()
	return nil
}

// namedLocked takes turn as the running turn, once codex has named it, by
// turn/started or by its reply to turn/start, whichever comes first, and
// tells the service the turn has started. a turn named already, or ended,
// is not named again. r.mu is held across the sink call, so the reader
// cannot mark the turn ended, and report its end, before its start has
// reached the service. callers hold r.mu.
func (r *runtime) namedLocked(turn string) {
	if turn == "" || r.activeTurn == turn || r.ended[turn] {
		return
	}
	r.activeTurn = turn
	r.sink.TurnStarted()
}

// Answer delivers the operator's answer. codex gives no acknowledgment for a
// response to its server request; one it already resolved is ignored by the
// server, and its serverRequest/resolved notice has removed the mapping here,
// so an answer after that reports ErrNotPending. an answer to the adapter's
// own question is steered into the turn that asked while that turn runs,
// settles from codex's reply, and never starts a turn.
func (r *runtime) Answer(requestID string, answer harness.Answer) error {
	r.mu.Lock()
	if r.stopping && harness.CarriesWork(answer) {
		r.mu.Unlock()
		return harness.ErrNotPending
	}
	if req, ok := r.requests[requestID]; ok {
		// the mapping goes and the answer is written in one step under the
		// lock, as claude's adapter sends its answer: a resolution codex
		// reported, read before this, settled the request already, and one
		// read after finds nothing left to settle.
		delete(r.requests, requestID)
		delete(r.byNative, nativeKey(req.id))
		err := r.conn.respond(req.id, responseFor(req, answer))
		r.mu.Unlock()
		return err
	}
	// a question leaves the adapter only once its steer is written: one
	// refused here stays for the abort that ends it, which reports it.
	q, ok := r.questions[requestID]
	if !ok || q.turn == "" || r.activeTurn != q.turn || r.interrupted[q.turn] {
		r.mu.Unlock()
		return harness.ErrNotPending
	}
	// the checks above and the steer's write are one hold of the lock, under
	// which Stopping and Interrupt mark a stop, so none lands between; the
	// wait for codex's reply holds none.
	steer := &turnSteerParams{ThreadID: r.threadID, ExpectedTurnID: q.turn, Input: text(answerText(answer.Answers, q.questions)), ClientUserMessageID: newID()}
	sent, err := r.conn.begin("turn/steer", steer)
	if err != nil {
		// nothing was written: the app-server has gone.
		r.mu.Unlock()
		dl.Debugf("steering an answer into turn '%s': %v", q.turn, err)
		return harness.ErrNotPending
	}
	delete(r.questions, requestID)
	r.steering.Add(1)
	r.mu.Unlock()
	// done once the settlement below has reached the sink.
	defer r.steering.Done()
	ctx, cancel := context.WithTimeout(context.Background(), r.steerTimeout)
	defer cancel()
	// a written steer is settled here, from codex's reply, before the exit
	// can be reported, so the exit's retirement never settles it another way.
	err = r.conn.wait(ctx, sent, nil)
	var refused *replyError
	switch {
	case err == nil:
		r.sink.Resolved(requestID, model.OutcomeAnswered)
		return nil
	case errors.As(err, &refused):
		// codex refused it, as when the turn has just ended: the answer is
		// not taken, and the message is answered in the reply box.
		dl.Debugf("steering an answer into turn '%s': %v", q.turn, err)
		r.sink.Resolved(requestID, model.OutcomeAborted)
		return harness.ErrNotPending
	default:
		// no reply before the deadline or the exit: codex may have taken it
		// or not, and it is never sent again.
		dl.Warnf("codex thread '%s': no reply to an answer steered into turn '%s': %v", r.NativeID(), q.turn, err)
		r.sink.Resolved(requestID, model.OutcomeUnconfirmed)
		return fmt.Errorf("Codex %w", harness.ErrUnconfirmed)
	}
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
	case methodElicitation:
		return elicitationResponse(req.elicitation, answer)
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

// turnStart builds a turn/start carrying the conversation's model and
// effort. codex takes each as an override that persists in the thread, and
// sending both on every turn keeps the thread on what the operator requests.
func (r *runtime) turnStart(threadID, uuid, body string) *turnStartParams {
	r.mu.Lock()
	defer r.mu.Unlock()
	return &turnStartParams{ThreadID: threadID, Input: text(body), ClientUserMessageID: uuid, Model: r.model, Effort: r.effort}
}

// SetModel stores the operator's choice for the next turn/start; nothing is
// sent until then, so it is always deferred. codex reports the change with
// thread/settings/updated when that turn starts.
func (r *runtime) SetModel(model, effort string) (bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.model, r.effort = model, effort
	return true, nil
}

// SetPermissionMode is refused: codex has no permission modes, and its approval
// policy and sandbox come from the operator's own configuration.
func (r *runtime) SetPermissionMode(string) error {
	return errors.New("codex has no permission modes; its approval policy comes from its own configuration")
}

// Interrupt stops the active turn. it marks the turn interrupted, which an
// answer checks before it steers, then aborts that turn's questions.
func (r *runtime) Interrupt() error {
	r.remember()
	r.mu.Lock()
	threadID, turnID := r.threadID, r.activeTurn
	if turnID != "" {
		r.interrupted[turnID] = true
	}
	r.mu.Unlock()
	// only the questions of the turn it stops: a question exists only for
	// the turn running, and one of a turn begun since is that turn's.
	r.abortQuestions(func(q *question) bool { return q.turn == turnID })
	if turnID == "" {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), callTimeout)
	defer cancel()
	return r.conn.call(ctx, "turn/interrupt", &turnInterruptParams{ThreadID: threadID, TurnID: turnID}, nil)
}

// Stopping fences a stop before it is carried out: it latches, so no answer
// that would let work go on is delivered after it, even one accepted before,
// and aborts the questions only such an answer could settle.
func (r *runtime) Stopping() {
	r.mu.Lock()
	r.stopping = true
	r.mu.Unlock()
	r.abortQuestions(allQuestions)
}

// Close ends input; the app-server exits when it sees the end, and closing is
// itself a clean cancel for a pending approval (capability report).
func (r *runtime) Close() error {
	r.remember()
	return r.conn.closeInput()
}

// abortQuestions takes the adapter's own questions match selects and reports
// them aborted, holding aborting throughout, so a release's fence never
// returns while another abort is still to be reported. callers hold no lock.
func (r *runtime) abortQuestions(match func(*question) bool) {
	r.aborting.Lock()
	defer r.aborting.Unlock()
	var ids []string
	r.mu.Lock()
	for id, q := range r.questions {
		if match(q) {
			ids = append(ids, id)
			delete(r.questions, id)
		}
	}
	r.mu.Unlock()
	for _, id := range ids {
		r.sink.Resolved(id, model.OutcomeAborted)
	}
}

func allQuestions(*question) bool { return true }

// watch reports the exit once the app-server is gone and every process seen
// under it at an interrupt or a close has gone too, or names what is still
// running when the grace runs out. the capability report saw a sandboxed
// command outlive the app-server after an interrupt.
func (r *runtime) watch() {
	<-r.conn.done
	r.mu.Lock()
	var seen []proc
	for _, p := range r.started {
		seen = append(seen, p)
	}
	unlisted := r.unlisted
	r.mu.Unlock()
	r.abortQuestions(allQuestions)
	// an answer whose steer was written settles itself, from codex's reply or
	// as unconfirmed. the exit is reported after it, so the service's
	// retirement of what is left never settles that question a second way.
	// every steer was counted before the holds of r.mu above.
	r.steering.Wait()
	r.mcpEnd()
	err := r.conn.err
	pids, scan := survivors(seen, survivorGrace)
	if scan == nil {
		scan = unlisted
	}
	if left := harness.Leftovers(pids, scan); left != nil {
		err = errors.Join(err, left)
	}
	r.sink.Exited(err)
}

// forThread is whether a frame belongs to this runtime's thread: one that
// names a thread is taken only when it is the thread confirmed, or, before
// codex has confirmed one, the thread a resume asked for. a fresh thread's
// frames before thread/start's reply are left, as nothing has asked for them
// yet. this is the per-frame check claude makes of every frame's session.
func (r *runtime) forThread(method string, params map[string]any) bool {
	named, ok := params["threadId"].(string)
	if !ok {
		return true
	}
	r.mu.Lock()
	want := r.threadID
	if want == "" {
		want = r.expected
	}
	r.mu.Unlock()
	if want != "" && named == want {
		return true
	}
	if want != "" {
		dl.Warnf("codex thread '%s': '%s' for another thread '%s' was left", want, method, named)
	}
	return false
}

func (r *runtime) notify(method string, params map[string]any) {
	if !r.forThread(method, params) {
		return
	}
	switch method {
	case "turn/started":
		n := &turnNotification{}
		if bind(n, params) {
			r.mu.Lock()
			r.namedLocked(n.Turn.ID)
			r.mu.Unlock()
		}
		// a server that died since the last list is not announced.
		r.refreshMCP()
	case "mcpServer/startupStatus/updated":
		// a server starting, ready or failed: the list says where each stands.
		r.refreshMCP()
	case "turn/completed":
		r.turnCompleted(params)
	case "item/started":
		n := &itemNotification{}
		if bind(n, params) && n.Item != nil {
			if it, err := bindItem(n.Item); err == nil {
				r.mu.Lock()
				switch it.Type {
				case "commandExecution":
					r.commands[it.ID] = &runningCommand{turn: n.TurnID, command: shellCommand(it.Command), started: time.Now().UTC()}
				case "fileChange":
					r.fileChanges[it.ID] = it.Changes
				case "mcpToolCall":
					r.mcpCalls = append(r.mcpCalls, mcpCall{itemID: it.ID, server: it.Server, tool: it.Tool})
				}
				r.mu.Unlock()
			}
		}
	case "item/completed":
		n := &itemNotification{}
		if bind(n, params) && n.Item != nil {
			if id, _ := n.Item["id"].(string); id != "" {
				r.mu.Lock()
				r.mcpCalls = slices.DeleteFunc(r.mcpCalls, func(c mcpCall) bool { return c.itemID == id })
				r.mu.Unlock()
			}
		}
		r.itemCompleted(params)
	case "item/agentMessage/delta":
		// streaming text of a turn codex has reported ended is dropped.
		n := &deltaNotification{}
		if bind(n, params) && n.Delta != "" && !r.hasEnded(n.TurnID) {
			r.sink.Delta(n.Delta)
		}
	case "thread/settings/updated":
		// codex's report of the model and effort it runs, as a turn's override
		// takes effect: the one writer of the live model and effort.
		n := &settingsUpdated{}
		if !bind(n, params) {
			return
		}
		reportedModel, _ := n.ThreadSettings["model"].(string)
		reportedEffort, _ := n.ThreadSettings["effort"].(string)
		r.mu.Lock()
		changed := (reportedModel != "" && reportedModel != r.liveModel) || (reportedEffort != "" && reportedEffort != r.liveEffort)
		if reportedModel != "" {
			r.liveModel = reportedModel
		}
		if reportedEffort != "" {
			r.liveEffort = reportedEffort
		}
		r.mu.Unlock()
		if changed {
			r.sink.Event(&model.Event{Kind: model.EventRuntime, Status: model.StatusModelChanged, Origin: model.OriginManaged, Text: "reported by codex", Facts: &model.RuntimeFacts{Model: reportedModel, Effort: reportedEffort}})
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
		// deltas. the items carry the conversation.
	}
}

// hasEnded is whether codex has reported turn ended. ended is written only
// on the reader, which every caller runs on.
func (r *runtime) hasEnded(turn string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.ended[turn]
}

func (r *runtime) turnCompleted(params map[string]any) {
	n := &turnNotification{}
	if !bind(n, params) {
		return
	}
	r.mu.Lock()
	r.ended[n.Turn.ID] = true
	if r.activeTurn == n.Turn.ID {
		r.activeTurn = ""
	}
	// a command the turn started and that is still running outlives it: a
	// terminal session codex keeps (unified exec), now background work.
	moved := false
	for _, c := range r.commands {
		if c.turn == n.Turn.ID && !c.background {
			c.background, moved = true, true
		}
	}
	var jobs []model.BackgroundJob
	if moved {
		jobs = r.backgroundLocked()
	}
	r.mu.Unlock()
	// no question outlives its turn, in any status: its message stays in the
	// conversation, and the reply box answers it. no question of this turn
	// can be raised meanwhile, since that too happens on the reader.
	r.abortQuestions(func(q *question) bool { return q.turn == n.Turn.ID })
	r.sink.Event(&model.Event{Kind: model.EventTurnEnd, Origin: model.OriginManaged, Status: turnStatus(n.Turn.Status), Text: n.Turn.Status, Raw: params})
	if moved {
		r.sink.Background(jobs)
	}
}

// runningCommand is a shell command codex started and has not reported
// completed; background once the turn that started it has ended.
type runningCommand struct {
	turn       string
	command    string
	started    time.Time
	background bool
}

// backgroundLocked is the background list, oldest first. callers hold r.mu.
func (r *runtime) backgroundLocked() []model.BackgroundJob {
	jobs := []model.BackgroundJob{}
	for id, c := range r.commands {
		if c.background {
			jobs = append(jobs, model.BackgroundJob{ID: id, Description: c.command, Kind: "command", StartedAt: c.started})
		}
	}
	slices.SortFunc(jobs, func(a, b model.BackgroundJob) int {
		if c := a.StartedAt.Compare(b.StartedAt); c != 0 {
			return c
		}
		return strings.Compare(a.ID, b.ID)
	})
	return jobs
}

// shellCommand is the command a shell item ran, without the login-shell
// wrapper codex puts around it ("/bin/bash -lc 'sleep 30'"), as codex's own
// background-terminal list shows it.
func shellCommand(command string) string {
	if m := shellWrapper.FindStringSubmatch(command); m != nil {
		if m[1] != "" {
			return m[1]
		}
		return m[2]
	}
	return command
}

var shellWrapper = regexp.MustCompile(`^/bin/(?:ba|z)?sh -l?c (?:'(.*)'|"(.*)")$`)

func (r *runtime) itemCompleted(params map[string]any) {
	n := &itemNotification{}
	if !bind(n, params) || n.Item == nil {
		return
	}
	if id, _ := n.Item["id"].(string); id != "" {
		r.mu.Lock()
		c := r.commands[id]
		delete(r.commands, id)
		var jobs []model.BackgroundJob
		if c != nil && c.background {
			jobs = r.backgroundLocked()
		}
		r.mu.Unlock()
		if c != nil && c.background {
			// a background command finished after its turn ended: its call
			// and result, after their turn.
			for _, ev := range backgroundFinished(n.Item) {
				r.sink.Event(ev)
			}
			r.sink.Background(jobs)
			return
		}
	}
	evs, err := mapItem(n.Item)
	if err != nil {
		dl.Warnf("codex thread '%s': %v", r.NativeID(), err)
		return
	}
	it, err := bindItem(n.Item)
	asks := err == nil && it.Type == "agentMessage" && len(it.Questions) > 0
	// an item of a turn codex has reported ended is output after its turn:
	// recorded, and a question among them is a message like any other.
	after := r.hasEnded(n.TurnID)
	for _, ev := range evs {
		ev.Origin = model.OriginManaged
		r.sink.Event(ev)
	}
	if !asks || after {
		return
	}
	// hold r.mu across the sink call, so an answer cannot arrive before the
	// question is known here.
	r.mu.Lock()
	defer r.mu.Unlock()
	if n.TurnID == "" || n.TurnID != r.activeTurn || r.interrupted[n.TurnID] || r.stopping {
		// only the running turn takes an answer, and not once the operator
		// interrupted it or a stop is fenced. the question is the adapter's
		// own, from the agent's text: nothing waits on it.
		return
	}
	id := r.sink.Request(model.RequestQuestion, questionPayload(it.Questions))
	r.questions[id] = &question{turn: n.TurnID, questions: it.Questions}
}

func (r *runtime) request(id any, method string, params map[string]any) {
	if !r.forThread(method, params) {
		return
	}
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
	case methodElicitation:
		p := &elicitationParams{}
		if !bind(p, params) {
			r.unanswerable(fmt.Sprintf("codex asked '%s' in a form mega cannot read; interrupt the turn to go on", method), params)
			return
		}
		e, k, pl, ok := r.classify(p)
		if !ok {
			r.unanswerable(unanswerableElicitation(p), params)
			return
		}
		req.elicitation, kind, payload = e, k, pl
	default:
		r.unanswerable(fmt.Sprintf("codex asked '%s', which mega cannot answer; interrupt the turn to go on", method), params)
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	svc := r.sink.Request(kind, payload)
	r.requests[svc] = req
	r.byNative[nativeKey(id)] = svc
}

// unanswerable records a server request mega cannot put to the operator.
// it is not answered either way on the operator's behalf: the turn waits, and
// the record says so, until the operator interrupts it.
func (r *runtime) unanswerable(text string, params map[string]any) {
	r.sink.Event(&model.Event{Kind: model.EventNative, Origin: model.OriginManaged, Text: text, Raw: params})
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

// backgroundFinished is the record of a background command that completed
// after its turn: the usual tool call and result, which open no turn, since
// only codex's start does, and count as unread at once, the way claude's
// woken turn leaves its reply unread. they carry the native identities a
// history import maps the same item to (mapItem), so a resume recognizes
// them rather than recording the command again.
func backgroundFinished(item map[string]any) []*model.Event {
	evs, err := mapItem(item)
	if err != nil {
		return nil
	}
	for _, ev := range evs {
		ev.Origin = model.OriginManaged
	}
	return evs
}
