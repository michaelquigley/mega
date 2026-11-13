package codex

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/michaelquigley/mega/internal/harness"
	"github.com/michaelquigley/mega/internal/model"
)

type recordingSink struct {
	mu       sync.Mutex
	events   []*model.Event
	deltas   []string
	requests []string
	payloads []map[string]any
	resolved []string
	// background holds every list reported, in order; mcp every MCP list.
	background [][]model.BackgroundJob
	mcp        [][]model.MCPServer
	next       int
}

func (s *recordingSink) MCPServers(servers []model.MCPServer) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.mcp = append(s.mcp, servers)
}

func (s *recordingSink) Background(jobs []model.BackgroundJob) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.background = append(s.background, jobs)
}

func (s *recordingSink) Event(ev *model.Event) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.events = append(s.events, ev)
}

func (s *recordingSink) Delta(text string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.deltas = append(s.deltas, text)
}

func (s *recordingSink) Request(kind string, payload any) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.next++
	id := fmt.Sprintf("svc-%d", s.next)
	s.requests = append(s.requests, kind+":"+id)
	p, _ := payload.(map[string]any)
	s.payloads = append(s.payloads, p)
	return id
}

func (s *recordingSink) Resolved(id, outcome string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.resolved = append(s.resolved, id+":"+outcome)
}

func (s *recordingSink) Exited(error) {}

// wire captures what the runtime writes to the app-server.
type wire struct {
	mu    sync.Mutex
	lines chan string
}

func (w *wire) Write(p []byte) (int, error) {
	for _, line := range strings.Split(strings.TrimSpace(string(p)), "\n") {
		w.lines <- line
	}
	return len(p), nil
}

func (w *wire) Close() error { return nil }

func testRuntime() (*runtime, *recordingSink, *wire) {
	sink := &recordingSink{}
	w := &wire{lines: make(chan string, 64)}
	r := newRuntime(sink)
	r.conn = &conn{stdin: w, calls: make(map[int64]chan callResult), done: make(chan struct{})}
	r.threadID = "thread-1"
	// a chosen model and effort, so a turn/start needs no config/read first;
	// the inherit tests clear them.
	r.cwd, r.model, r.effort = "/f", "gpt-test", "medium"
	// the MCP lists are tested on their own (mcp_test.go); one marked in
	// flight keeps a turn's start from adding a list to the frames these
	// tests expect.
	r.mcpBusy = true
	return r, sink, w
}

// sent decodes the next frame the runtime wrote. test-only inspection of
// emitted bytes.
func (w *wire) sent(t *testing.T) map[string]any {
	t.Helper()
	select {
	case line := <-w.lines:
		frame := map[string]any{}
		if err := json.Unmarshal([]byte(line), &frame); err != nil {
			t.Fatalf("unreadable frame %q: %v", line, err)
		}
		return frame
	case <-time.After(2 * time.Second):
		t.Fatal("nothing was sent")
		return nil
	}
}

func (w *wire) quiet(t *testing.T) {
	t.Helper()
	select {
	case line := <-w.lines:
		t.Fatalf("unexpected frame %s", line)
	case <-time.After(20 * time.Millisecond):
	}
}

func (r *runtime) feed(line string) {
	r.conn.dispatch([]byte(line), r)
}

// reply answers the call the runtime just made, as the app-server would.
func (r *runtime) reply(t *testing.T, w *wire, method, result string) map[string]any {
	t.Helper()
	frame := w.sent(t)
	if frame["method"] != method {
		t.Fatalf("sent %v, want %s", frame["method"], method)
	}
	r.feed(fmt.Sprintf(`{"id":%v,"result":%s}`, frame["id"], result))
	return frame
}

func summary(evs []*model.Event) []string {
	var out []string
	for _, ev := range evs {
		out = append(out, strings.Join([]string{ev.Kind, ev.Status, ev.NativeID, ev.Tool, ev.ToolUseID, ev.Text}, "|"))
	}
	return out
}

func TestItemsMapToEvents(t *testing.T) {
	r, sink, _ := testRuntime()
	r.feed(`{"method":"turn/started","params":{"threadId":"thread-1","turn":{"id":"turn-1","status":"inProgress","items":[]}}}`)
	r.feed(`{"method":"item/completed","params":{"threadId":"thread-1","turnId":"turn-1","item":{"type":"userMessage","id":"u-9","clientId":"svc-uuid","content":[{"type":"text","text":"Reply with ALPHA"}]}}}`)
	r.feed(`{"method":"item/agentMessage/delta","params":{"threadId":"thread-1","turnId":"turn-1","itemId":"a-1","delta":"ROUND"}}`)
	r.feed(`{"method":"item/completed","params":{"threadId":"thread-1","turnId":"turn-1","item":{"type":"agentMessage","id":"a-1","text":"ROUNDHOUSE-MARKER-7f3a ALPHA-1a2b"}}}`)
	r.feed(`{"method":"item/completed","params":{"threadId":"thread-1","turnId":"turn-1","item":{"type":"commandExecution","id":"c-1","command":"printf x > f","cwd":"/f","commandActions":[],"status":"failed","exitCode":1,"aggregatedOutput":"denied"}}}`)
	r.feed(`{"method":"item/completed","params":{"threadId":"thread-1","turnId":"turn-1","item":{"type":"reasoning","id":"r-1"}}}`)
	r.feed(`{"method":"turn/completed","params":{"threadId":"thread-1","turn":{"id":"turn-1","status":"interrupted","items":[]}}}`)

	want := []string{
		"user||svc-uuid|||Reply with ALPHA",
		"assistant||a-1|||ROUNDHOUSE-MARKER-7f3a ALPHA-1a2b",
		"tool_use||c-1|shell|c-1|shell printf x > f",
		"tool_result|error|c-1/result||c-1|denied",
		"native||r-1|||reasoning",
		"turn_end|interrupted||||interrupted",
	}
	if got := summary(sink.events); strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("events:\n%s\nwant:\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
	if strings.Join(sink.deltas, "") != "ROUND" {
		t.Errorf("deltas = %v", sink.deltas)
	}
	if r.activeTurn != "" {
		t.Errorf("active turn after completion = %q", r.activeTurn)
	}
}

func TestApprovalIsAnsweredOnceByItsServerID(t *testing.T) {
	r, sink, w := testRuntime()
	r.feed(`{"id":0,"method":"item/commandExecution/requestApproval","params":{"threadId":"thread-1","turnId":"turn-1","itemId":"c-1","startedAtMs":1,"command":"printf x > f","cwd":"/f","reason":"write outside the sandbox"}}`)
	if strings.Join(sink.requests, ",") != "permission:svc-1" {
		t.Fatalf("requests = %v", sink.requests)
	}
	payload := sink.payloads[0]
	if payload["tool"] != "shell" || payload["description"] != "write outside the sandbox" || payload["input"].(map[string]any)["command"] != "printf x > f" {
		t.Errorf("payload = %v", payload)
	}
	if err := r.Answer("svc-1", harness.Answer{Decision: harness.DecisionAllow}); err != nil {
		t.Fatal(err)
	}
	frame := w.sent(t)
	if frame["id"] != float64(0) || frame["result"].(map[string]any)["decision"] != "accept" {
		t.Errorf("response = %v", frame)
	}
	if err := r.Answer("svc-1", harness.Answer{Decision: harness.DecisionAllow}); !errors.Is(err, harness.ErrNotPending) {
		t.Errorf("second answer: %v", err)
	}
	w.quiet(t)
}

func TestResolvedByServerIsAbortedAndNotAnswered(t *testing.T) {
	r, sink, w := testRuntime()
	r.feed(`{"id":1,"method":"item/fileChange/requestApproval","params":{"threadId":"thread-1","turnId":"turn-1","itemId":"f-1","startedAtMs":1}}`)
	r.feed(`{"method":"serverRequest/resolved","params":{"threadId":"thread-1","requestId":1}}`)
	if strings.Join(sink.resolved, ",") != "svc-1:aborted" {
		t.Errorf("resolved = %v", sink.resolved)
	}
	if err := r.Answer("svc-1", harness.Answer{Decision: harness.DecisionAllow}); !errors.Is(err, harness.ErrNotPending) {
		t.Errorf("late answer: %v", err)
	}
	w.quiet(t)
}

func TestPermissionsGrantExactlyWhatWasAskedForTheTurn(t *testing.T) {
	r, _, w := testRuntime()
	r.feed(`{"id":2,"method":"item/permissions/requestApproval","params":{"threadId":"thread-1","turnId":"turn-1","itemId":"p-1","startedAtMs":1,"cwd":"/f","permissions":{"network":{"enabled":true}}}}`)
	r.feed(`{"id":3,"method":"item/permissions/requestApproval","params":{"threadId":"thread-1","turnId":"turn-1","itemId":"p-2","startedAtMs":1,"cwd":"/f","permissions":{"network":{"enabled":true}}}}`)
	if err := r.Answer("svc-1", harness.Answer{Decision: harness.DecisionAllow}); err != nil {
		t.Fatal(err)
	}
	allowed := w.sent(t)["result"].(map[string]any)
	if allowed["scope"] != "turn" || allowed["permissions"].(map[string]any)["network"] == nil {
		t.Errorf("allowed = %v", allowed)
	}
	if err := r.Answer("svc-2", harness.Answer{Decision: harness.DecisionDeny}); err != nil {
		t.Fatal(err)
	}
	denied := w.sent(t)["result"].(map[string]any)
	if denied["scope"] != "turn" || len(denied["permissions"].(map[string]any)) != 0 {
		t.Errorf("denied = %v", denied)
	}
}

func askQuestion(r *runtime) {
	r.feed(`{"method":"turn/started","params":{"threadId":"thread-1","turn":{"id":"turn-1","status":"inProgress","items":[]}}}`)
	r.feed(`{"method":"item/completed","params":{"threadId":"thread-1","turnId":"turn-1","item":{"type":"agentMessage","id":"a-1","text":"Which color?","delivery":"async","questions":[{"title":"Which color do you prefer?","options":["Red","Blue"]}]}}}`)
}

func TestAsyncQuestionIsSteeredIntoItsActiveTurn(t *testing.T) {
	r, sink, w := testRuntime()
	askQuestion(r)
	if strings.Join(sink.requests, ",") != "question:svc-1" {
		t.Fatalf("requests = %v", sink.requests)
	}
	questions := sink.payloads[0]["questions"].([]any)
	if q := questions[0].(map[string]any); q["question"] != "Which color do you prefer?" || len(q["options"].([]any)) != 2 {
		t.Errorf("payload = %v", sink.payloads[0])
	}
	result := make(chan error, 1)
	go func() {
		result <- r.Answer("svc-1", harness.Answer{Answers: map[string]string{"Which color do you prefer?": "Blue"}})
	}()
	frame := r.reply(t, w, "turn/steer", `{"turnId":"turn-1"}`)
	params := frame["params"].(map[string]any)
	if params["expectedTurnId"] != "turn-1" || params["input"].([]any)[0].(map[string]any)["text"] != "Blue" || params["clientUserMessageId"] == "" {
		t.Errorf("steer = %v", params)
	}
	if err := <-result; err != nil {
		t.Fatal(err)
	}
	if err := r.Answer("svc-1", harness.Answer{Answers: map[string]string{"Which color do you prefer?": "Red"}}); !errors.Is(err, harness.ErrNotPending) {
		t.Errorf("second answer: %v", err)
	}
}

func TestAsyncQuestionDiesWithAnInterruptedOrFailedTurn(t *testing.T) {
	for _, status := range []string{"interrupted", "failed"} {
		r, sink, w := testRuntime()
		askQuestion(r)
		r.feed(`{"method":"turn/completed","params":{"threadId":"thread-1","turn":{"id":"turn-1","status":"` + status + `","items":[]}}}`)
		if strings.Join(sink.resolved, ",") != "svc-1:aborted" {
			t.Errorf("%s: resolved = %v", status, sink.resolved)
		}
		if err := r.Answer("svc-1", harness.Answer{Answers: map[string]string{"Which color do you prefer?": "Blue"}}); !errors.Is(err, harness.ErrNotPending) {
			t.Errorf("%s: late answer: %v", status, err)
		}
		w.quiet(t)
	}
}

func TestAsyncQuestionOutlivesACompletedTurnAndStartsTheNext(t *testing.T) {
	r, sink, w := testRuntime()
	askQuestion(r)
	r.feed(`{"method":"turn/completed","params":{"threadId":"thread-1","turn":{"id":"turn-1","status":"completed","items":[]}}}`)
	if len(sink.resolved) != 0 {
		t.Fatalf("a question from a completed turn was resolved: %v", sink.resolved)
	}
	result := make(chan error, 1)
	go func() {
		result <- r.Answer("svc-1", harness.Answer{Answers: map[string]string{"Which color do you prefer?": "Blue"}})
	}()
	frame := r.reply(t, w, "turn/start", `{"turn":{"id":"turn-2","status":"inProgress","items":[]}}`)
	if text := frame["params"].(map[string]any)["input"].([]any)[0].(map[string]any)["text"]; text != "Blue" {
		t.Errorf("turn/start text = %v", text)
	}
	if err := <-result; err != nil {
		t.Fatal(err)
	}
}

func TestInterruptAbortsQuestionsAtOnce(t *testing.T) {
	r, sink, w := testRuntime()
	askQuestion(r)
	result := make(chan error, 1)
	go func() { result <- r.Interrupt() }()
	frame := r.reply(t, w, "turn/interrupt", `{}`)
	if frame["params"].(map[string]any)["turnId"] != "turn-1" {
		t.Errorf("interrupt = %v", frame)
	}
	if err := <-result; err != nil {
		t.Fatal(err)
	}
	if strings.Join(sink.resolved, ",") != "svc-1:aborted" {
		t.Errorf("resolved = %v", sink.resolved)
	}
	if err := r.Answer("svc-1", harness.Answer{Answers: map[string]string{"Which color do you prefer?": "Blue"}}); !errors.Is(err, harness.ErrNotPending) {
		t.Errorf("late answer: %v", err)
	}
}

func TestUnknownServerRequestIsNeitherAnsweredNorHidden(t *testing.T) {
	r, sink, w := testRuntime()
	r.feed(`{"id":4,"method":"item/tool/somethingNew","params":{"threadId":"thread-1"}}`)
	// an elicitation the adapter cannot even read is recorded the same way.
	r.feed(`{"id":5,"method":"mcpServer/elicitation/request","params":{"threadId":"thread-1"}}`)
	w.quiet(t)
	if len(sink.requests) != 0 || len(sink.events) != 2 || !strings.Contains(sink.events[0].Text, "item/tool/somethingNew") || !strings.Contains(sink.events[1].Text, "mcpServer/elicitation/request") {
		t.Errorf("requests %v, events %v", sink.requests, summary(sink.events))
	}
}

func TestPromptCarriesTheServiceIdentity(t *testing.T) {
	r, _, w := testRuntime()
	result := make(chan error, 1)
	go func() { result <- r.Prompt("svc-uuid", "Reply with ALPHA") }()
	frame := r.reply(t, w, "turn/start", `{"turn":{"id":"turn-7","status":"inProgress","items":[]}}`)
	params := frame["params"].(map[string]any)
	if params["clientUserMessageId"] != "svc-uuid" || params["threadId"] != "thread-1" {
		t.Errorf("turn/start = %v", params)
	}
	if err := <-result; err != nil {
		t.Fatal(err)
	}
	if r.activeTurn != "turn-7" {
		t.Errorf("active turn = %q", r.activeTurn)
	}
}

func TestStartRequestsCarryNoPermissionOverride(t *testing.T) {
	for _, params := range []any{&threadStartParams{Cwd: "/f"}, &threadResumeParams{ThreadID: "t", Cwd: "/f"}} {
		r, _, w := testRuntime()
		go func() { _ = r.conn.call(t.Context(), "thread/start", params, nil) }()
		frame := w.sent(t)
		p := frame["params"].(map[string]any)
		for _, key := range []string{"approvalPolicy", "sandbox", "config", "permissions"} {
			if _, ok := p[key]; ok {
				t.Errorf("%T sends %s", params, key)
			}
		}
	}
}

func TestQuestionFromAnInterruptedTurnNeverWaits(t *testing.T) {
	r, sink, w := testRuntime()
	r.feed(`{"method":"turn/started","params":{"threadId":"thread-1","turn":{"id":"turn-1","status":"inProgress","items":[]}}}`)
	result := make(chan error, 1)
	go func() { result <- r.Interrupt() }()
	r.reply(t, w, "turn/interrupt", `{}`)
	if err := <-result; err != nil {
		t.Fatal(err)
	}
	// the turn had already produced a question and then finished before the
	// interrupt landed.
	r.feed(`{"method":"item/completed","params":{"threadId":"thread-1","turnId":"turn-1","item":{"type":"agentMessage","id":"a-1","text":"Which color?","questions":[{"title":"Which color?","options":["Red","Blue"]}]}}}`)
	r.feed(`{"method":"turn/completed","params":{"threadId":"thread-1","turn":{"id":"turn-1","status":"completed","items":[]}}}`)
	if len(sink.requests) != 0 {
		t.Errorf("a question from the interrupted turn was raised: %v", sink.requests)
	}
	if err := r.Answer("svc-1", harness.Answer{Answers: map[string]string{"Which color?": "Blue"}}); !errors.Is(err, harness.ErrNotPending) {
		t.Errorf("answer: %v", err)
	}
	w.quiet(t)
}

// an answer is waiting on turn/steer when its turn completes and the operator
// interrupts; the steer then fails. the answer must not start a new turn.
func TestAnswerInFlightNeverStartsATurnAcrossAnInterrupt(t *testing.T) {
	r, _, w := testRuntime()
	askQuestion(r)
	result := make(chan error, 1)
	go func() {
		result <- r.Answer("svc-1", harness.Answer{Answers: map[string]string{"Which color do you prefer?": "Blue"}})
	}()
	steer := w.sent(t)
	if steer["method"] != "turn/steer" {
		t.Fatalf("sent %v", steer["method"])
	}
	r.feed(`{"method":"turn/completed","params":{"threadId":"thread-1","turn":{"id":"turn-1","status":"completed","items":[]}}}`)
	if err := r.Interrupt(); err != nil { // no active turn: nothing is sent
		t.Fatal(err)
	}
	r.feed(fmt.Sprintf(`{"id":%v,"error":{"code":-32600,"message":"no active turn"}}`, steer["id"]))
	if err := <-result; !errors.Is(err, harness.ErrNotPending) {
		t.Errorf("answer: %v", err)
	}
	w.quiet(t)
}

// a stop that lands while the answer's turn is being started interrupts that
// turn as soon as it exists.
func TestStopDuringAnswerTurnStartInterruptsIt(t *testing.T) {
	r, _, w := testRuntime()
	askQuestion(r)
	r.feed(`{"method":"turn/completed","params":{"threadId":"thread-1","turn":{"id":"turn-1","status":"completed","items":[]}}}`)
	result := make(chan error, 1)
	go func() {
		result <- r.Answer("svc-1", harness.Answer{Answers: map[string]string{"Which color do you prefer?": "Blue"}})
	}()
	start := w.sent(t)
	if start["method"] != "turn/start" {
		t.Fatalf("sent %v", start["method"])
	}
	if err := r.Interrupt(); err != nil {
		t.Fatal(err)
	}
	r.feed(fmt.Sprintf(`{"id":%v,"result":{"turn":{"id":"turn-2","status":"inProgress","items":[]}}}`, start["id"]))
	interrupt := r.reply(t, w, "turn/interrupt", `{}`)
	if interrupt["params"].(map[string]any)["turnId"] != "turn-2" {
		t.Errorf("interrupted %v", interrupt["params"])
	}
	if err := <-result; !errors.Is(err, harness.ErrNotPending) {
		t.Errorf("answer: %v", err)
	}
}
