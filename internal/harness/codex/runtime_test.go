package codex

import (
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
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
	// atExit is what had been resolved when the exit was reported.
	exited bool
	atExit []string
	// turnStarts holds, for each turn start declared, how many events had
	// been reported before it.
	turnStarts []int
	// onResolved, when set, runs as a resolution is reported, before it is
	// recorded.
	onResolved func()
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

// TurnStarted counts the turn starts the runtime declared, in order among
// the events: each is a marker in turnStarts at the number of events then.
func (s *recordingSink) TurnStarted() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.turnStarts = append(s.turnStarts, len(s.events))
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
	hook := s.onResolved
	s.mu.Unlock()
	if hook != nil {
		hook()
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.resolved = append(s.resolved, id+":"+outcome)
}

func (s *recordingSink) Exited(error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.exited = true
	s.atExit = slices.Clone(s.resolved)
}

// wire captures what the runtime writes to the app-server.
type wire struct {
	mu    sync.Mutex
	lines chan string
	// onWrite, when set, runs as a frame is written, before it is taken.
	onWrite func()
}

func (w *wire) Write(p []byte) (int, error) {
	if w.onWrite != nil {
		w.onWrite()
	}
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
	// the conversation's request, which every turn/start passes.
	r.model, r.effort = "gpt-test", "medium"
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
		t.Fatalf("payload = %v", payload)
	}
	if err := r.Answer("svc-1", harness.Answer{Decision: harness.DecisionAllow}); err != nil {
		t.Fatal(err)
	}
	frame := w.sent(t)
	if frame["id"] != float64(0) || frame["result"].(map[string]any)["decision"] != "accept" {
		t.Errorf("response = %v", frame)
	}
	if err := r.Answer("svc-1", harness.Answer{Decision: harness.DecisionDeny}); !errors.Is(err, harness.ErrNotPending) {
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
	if err := r.Answer("svc-1", harness.Answer{Decision: harness.DecisionDeny}); !errors.Is(err, harness.ErrNotPending) {
		t.Errorf("late answer: %v", err)
	}
	w.quiet(t)
}

func TestPermissionsGrantExactlyWhatWasAskedForTheTurn(t *testing.T) {
	r, sink, w := testRuntime()
	r.feed(`{"id":2,"method":"item/permissions/requestApproval","params":{"threadId":"thread-1","turnId":"turn-1","itemId":"p-1","startedAtMs":1,"cwd":"/f","permissions":{"network":{"enabled":true}}}}`)
	r.feed(`{"id":3,"method":"item/permissions/requestApproval","params":{"threadId":"thread-1","turnId":"turn-1","itemId":"p-2","startedAtMs":1,"cwd":"/f","permissions":{"network":{"enabled":true}}}}`)
	// the request allowed is the network grant the fixture asked for.
	if b, _ := json.Marshal(sink.payloads[0]); len(sink.payloads) != 2 || !strings.Contains(string(b), `"network"`) {
		t.Fatalf("the request is not the network grant: %s", b)
	}
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

// askQuestion raises the agent's question and looks at it: no test answers
// a question it has not seen raised as the one it means.
func askQuestion(t *testing.T, r *runtime) {
	t.Helper()
	raiseQuestion(r)
	s := r.sink.(*recordingSink)
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.payloads) == 0 {
		t.Fatal("no question was raised")
	}
	if b, _ := json.Marshal(s.payloads[len(s.payloads)-1]); !strings.Contains(string(b), `"question":"Which color do you prefer?"`) {
		t.Fatalf("the question raised is not the one meant: %s", b)
	}
}

// raiseQuestion feeds the agent's question as codex sends it.
func raiseQuestion(r *runtime) {
	r.feed(`{"method":"turn/started","params":{"threadId":"thread-1","turn":{"id":"turn-1","status":"inProgress","items":[]}}}`)
	r.feed(`{"method":"item/completed","params":{"threadId":"thread-1","turnId":"turn-1","item":{"type":"agentMessage","id":"a-1","text":"Which color?","delivery":"async","questions":[{"title":"Which color do you prefer?","options":["Red","Blue"]}]}}}`)
}

func TestAsyncQuestionIsSteeredIntoItsActiveTurn(t *testing.T) {
	r, sink, w := testRuntime()
	askQuestion(t, r)
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
		askQuestion(t, r)
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

func TestInterruptAbortsQuestionsAtOnce(t *testing.T) {
	r, sink, w := testRuntime()
	askQuestion(t, r)
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
	if err := r.Answer("svc-1", harness.Answer{Decision: harness.DecisionDeny}); !errors.Is(err, harness.ErrNotPending) {
		t.Errorf("answer: %v", err)
	}
	w.quiet(t)
}

// a question the agent asks after the stop is fenced is not raised: the
// fenced runtime would refuse its answer, so it would only wait in Needs you
// until the runtime exits.
func TestNoQuestionIsRaisedAfterTheStopIsFenced(t *testing.T) {
	r, sink, _ := testRuntime()
	r.Stopping()
	raiseQuestion(r)
	if len(sink.requests) != 0 {
		t.Errorf("a question was raised after the fence: %v", sink.requests)
	}
}

// a release ends an unanswered question as it fences: the question is
// aborted before Stopping returns, so none is left listed for the page while
// the interrupt is still on its way. a steer already written is left to
// settle from its reply.
func TestAReleaseAbortsUnansweredQuestionsAsItFences(t *testing.T) {
	r, sink, w := testRuntime()
	askQuestion(t, r)
	r.Stopping()
	sink.mu.Lock()
	resolved := slices.Clone(sink.resolved)
	sink.mu.Unlock()
	if strings.Join(resolved, ",") != "svc-1:aborted" {
		t.Errorf("when the fence returned, resolved = %v", resolved)
	}
	w.quiet(t)

	r, sink, w = testRuntime()
	askQuestion(t, r)
	result := make(chan error, 1)
	go func() {
		result <- r.Answer("svc-1", harness.Answer{Answers: map[string]string{"Which color do you prefer?": "Blue"}})
	}()
	steer := w.sent(t)
	if steer["method"] != "turn/steer" {
		t.Fatalf("sent %v", steer["method"])
	}
	r.Stopping()
	sink.mu.Lock()
	early := slices.Clone(sink.resolved)
	sink.mu.Unlock()
	if len(early) != 0 {
		t.Errorf("the fence settled a written steer: %v", early)
	}
	r.feed(fmt.Sprintf(`{"id":%v,"result":{"turnId":"turn-1"}}`, steer["id"]))
	if err := <-result; err != nil {
		t.Errorf("answer: %v", err)
	}
}

// a release's fence returns only once no abort of the adapter's own questions
// is still to be reported: whatever took them first, the release's other
// fence, an interrupt or the turn's end, a release never returns with one
// still listed.
func TestAFenceWaitsForEveryAbortUnderWay(t *testing.T) {
	for _, first := range []string{"fence", "interrupt", "turn end"} {
		t.Run(first, func(t *testing.T) {
			r, sink, w := testRuntime()
			askQuestion(t, r)
			reporting, release := make(chan struct{}), make(chan struct{})
			sink.mu.Lock()
			sink.onResolved = func() {
				close(reporting)
				<-release
			}
			sink.mu.Unlock()
			done := make(chan struct{})
			go func() {
				defer close(done)
				switch first {
				case "fence":
					r.Stopping()
				case "interrupt":
					if err := r.Interrupt(); err != nil {
						t.Error(err)
					}
				case "turn end":
					r.feed(`{"method":"turn/completed","params":{"threadId":"thread-1","turn":{"id":"turn-1","status":"completed","items":[]}}}`)
				}
			}()
			<-reporting
			fenced := make(chan struct{})
			go func() {
				r.Stopping()
				close(fenced)
			}()
			select {
			case <-fenced:
				t.Error("the fence returned before the abort under way was reported")
			case <-time.After(100 * time.Millisecond):
			}
			close(release)
			<-fenced
			if first == "interrupt" {
				r.reply(t, w, "turn/interrupt", `{}`)
			}
			<-done
			sink.mu.Lock()
			resolved := slices.Clone(sink.resolved)
			sink.mu.Unlock()
			if strings.Join(resolved, ",") != "svc-1:aborted" {
				t.Errorf("resolved = %v", resolved)
			}
		})
	}
}

// an interrupt marks its turn before it takes the turn's questions: an answer
// that reaches the adapter in between is refused, not steered into the turn
// being stopped.
func TestNoSteerIntoATurnBeingInterrupted(t *testing.T) {
	r, _, w := testRuntime()
	askQuestion(t, r)
	r.mu.Lock()
	r.interrupted["turn-1"] = true
	r.mu.Unlock()
	result := make(chan error, 1)
	go func() {
		result <- r.Answer("svc-1", harness.Answer{Answers: map[string]string{"Which color do you prefer?": "Blue"}})
	}()
	select {
	case err := <-result:
		if !errors.Is(err, harness.ErrNotPending) {
			t.Errorf("answer: %v", err)
		}
	case line := <-w.lines:
		t.Fatalf("an answer was steered into a turn being interrupted: %s", line)
	case <-time.After(2 * time.Second):
		t.Fatal("the answer did not return")
	}
	w.quiet(t)
}

// an answer the adapter refuses without writing a steer leaves its question
// to the abort that is under way: the turn's end, an interrupt or a release
// still takes it and reports it, so none of them is reported while its card
// is listed.
func TestARefusedAnswerLeavesItsQuestionToTheAbort(t *testing.T) {
	r, sink, w := testRuntime()
	askQuestion(t, r)
	// the turn's end has cleared the active turn, and not yet taken its
	// questions.
	r.mu.Lock()
	r.activeTurn = ""
	r.mu.Unlock()
	if err := r.Answer("svc-1", harness.Answer{Answers: map[string]string{"Which color do you prefer?": "Blue"}}); !errors.Is(err, harness.ErrNotPending) {
		t.Errorf("answer: %v", err)
	}
	w.quiet(t)
	r.abortQuestions(allQuestions)
	sink.mu.Lock()
	resolved := slices.Clone(sink.resolved)
	sink.mu.Unlock()
	if strings.Join(resolved, ",") != "svc-1:aborted" {
		t.Errorf("the abort under way reported %v", resolved)
	}
}

// an interrupt aborts the questions of the turn it stops, and no other: a
// question of a turn that started after it caught its turn is left to that
// turn.
func TestAnInterruptAbortsOnlyItsTurnsQuestions(t *testing.T) {
	r, sink, w := testRuntime()
	r.feed(`{"method":"turn/started","params":{"threadId":"thread-1","turn":{"id":"turn-0","status":"inProgress","items":[]}}}`)
	r.mu.Lock()
	r.questions["svc-9"] = &question{turn: "turn-9"}
	r.mu.Unlock()
	result := make(chan error, 1)
	go func() { result <- r.Interrupt() }()
	if frame := r.reply(t, w, "turn/interrupt", `{}`); frame["params"].(map[string]any)["turnId"] != "turn-0" {
		t.Errorf("interrupted %v", frame["params"])
	}
	if err := <-result; err != nil {
		t.Fatal(err)
	}
	sink.mu.Lock()
	resolved := slices.Clone(sink.resolved)
	sink.mu.Unlock()
	if len(resolved) != 0 {
		t.Errorf("an interrupt of turn-0 aborted %v", resolved)
	}
}

// once the adapter has fenced a stop while a question is pending in its
// running turn, an answer is refused and no steer is sent: a release fences
// when Stopping latches, before its interrupt, and an interrupt when it takes
// the turn's questions.
func TestNoSteerIsSentOnceAStopIsFenced(t *testing.T) {
	for _, stop := range []string{"release", "interrupt"} {
		t.Run(stop, func(t *testing.T) {
			r, _, w := testRuntime()
			askQuestion(t, r)
			switch stop {
			case "release":
				r.Stopping()
			case "interrupt":
				result := make(chan error, 1)
				go func() { result <- r.Interrupt() }()
				r.reply(t, w, "turn/interrupt", `{}`)
				if err := <-result; err != nil {
					t.Fatal(err)
				}
			}
			result := make(chan error, 1)
			go func() {
				result <- r.Answer("svc-1", harness.Answer{Answers: map[string]string{"Which color do you prefer?": "Blue"}})
			}()
			select {
			case err := <-result:
				if !errors.Is(err, harness.ErrNotPending) {
					t.Errorf("answer after the fence: %v", err)
				}
			case line := <-w.lines:
				t.Fatalf("after the fence, %s was sent", line)
			case <-time.After(2 * time.Second):
				t.Fatal("the answer did not return")
			}
			w.quiet(t)
		})
	}
}

// a turn that completes with its question unanswered aborts the question, as
// an interrupted or failed one does: no question outlives its turn.
func TestAsyncQuestionDiesWithACompletedTurn(t *testing.T) {
	r, sink, w := testRuntime()
	askQuestion(t, r)
	r.feed(`{"method":"turn/completed","params":{"threadId":"thread-1","turn":{"id":"turn-1","status":"completed","items":[]}}}`)
	if strings.Join(sink.resolved, ",") != "svc-1:aborted" {
		t.Errorf("resolved = %v", sink.resolved)
	}
	w.quiet(t)
}

// an answer to a question whose turn has ended is refused, and nothing is
// sent: it never starts the next turn.
func TestAnAnswerAfterItsTurnHasEndedIsRefused(t *testing.T) {
	for _, status := range []string{"completed", "interrupted", "failed"} {
		t.Run(status, func(t *testing.T) {
			r, _, w := testRuntime()
			askQuestion(t, r)
			r.feed(`{"method":"turn/completed","params":{"threadId":"thread-1","turn":{"id":"turn-1","status":"` + status + `","items":[]}}}`)
			result := make(chan error, 1)
			go func() {
				result <- r.Answer("svc-1", harness.Answer{Answers: map[string]string{"Which color do you prefer?": "Blue"}})
			}()
			select {
			case err := <-result:
				if !errors.Is(err, harness.ErrNotPending) {
					t.Errorf("answer: %v", err)
				}
			case line := <-w.lines:
				t.Fatalf("an answer after its turn sent %s", line)
			case <-time.After(2 * time.Second):
				t.Fatal("the answer did not return")
			}
			w.quiet(t)
		})
	}
}

// a message with questions that arrives once its turn has ended raises no
// request and starts no turn: it is recorded as a message, and the reply box
// answers it like any message.
func TestAQuestionAfterItsTurnIsAMessage(t *testing.T) {
	for _, status := range []string{"completed", "interrupted", "failed"} {
		t.Run(status, func(t *testing.T) {
			r, sink, w := testRuntime()
			r.feed(`{"method":"turn/started","params":{"threadId":"thread-1","turn":{"id":"turn-1","status":"inProgress","items":[]}}}`)
			r.feed(`{"method":"turn/completed","params":{"threadId":"thread-1","turn":{"id":"turn-1","status":"` + status + `","items":[]}}}`)
			r.feed(`{"method":"item/completed","params":{"threadId":"thread-1","turnId":"turn-1","item":{"type":"agentMessage","id":"a-1","text":"Which color?","delivery":"async","questions":[{"title":"Which color do you prefer?","options":["Red","Blue"]}]}}}`)
			if len(sink.requests) != 0 {
				t.Errorf("a question after its turn was raised: %v", sink.requests)
			}
			last := sink.events[len(sink.events)-1]
			if last.Kind != model.EventAssistant || last.Text != "Which color?" {
				t.Errorf("the message is recorded as %+v", last)
			}
			if len(sink.turnStarts) != 1 {
				t.Errorf("turn starts declared at %v, want only the turn's own", sink.turnStarts)
			}
			w.quiet(t)
		})
	}
}

// streaming text codex sends for a turn it has reported ended is never the
// running turn's: it is dropped, and the completed item records the whole
// message.
func TestStreamingTextOfAnEndedTurnIsDropped(t *testing.T) {
	r, sink, _ := testRuntime()
	r.feed(`{"method":"turn/started","params":{"threadId":"thread-1","turn":{"id":"turn-1","status":"inProgress","items":[]}}}`)
	r.feed(`{"method":"turn/completed","params":{"threadId":"thread-1","turn":{"id":"turn-1","status":"completed","items":[]}}}`)
	r.feed(`{"method":"turn/started","params":{"threadId":"thread-1","turn":{"id":"turn-2","status":"inProgress","items":[]}}}`)
	r.feed(`{"method":"item/agentMessage/delta","params":{"threadId":"thread-1","turnId":"turn-1","itemId":"a-1","delta":"LATE"}}`)
	r.feed(`{"method":"item/agentMessage/delta","params":{"threadId":"thread-1","turnId":"turn-2","itemId":"a-2","delta":"LIVE"}}`)
	if got := strings.Join(sink.deltas, ""); got != "LIVE" {
		t.Errorf("streamed %q, want only the running turn's", got)
	}
}

// a card is raised only for a question of the turn that is running, the one
// its answer can be steered into: a question naming another turn, or arriving
// with no turn known to run, is recorded as the message it is and raises none.
func TestOnlyTheRunningTurnsQuestionRaisesACard(t *testing.T) {
	question := func(turn string) string {
		return `{"method":"item/completed","params":{"threadId":"thread-1","turnId":"` + turn + `","item":{"type":"agentMessage","id":"a-` + turn + `","text":"Which color?","questions":[{"title":"Which color do you prefer?","options":["Red","Blue"]}]}}}`
	}
	r, sink, w := testRuntime()
	r.feed(question("turn-1"))
	r.feed(`{"method":"turn/started","params":{"threadId":"thread-1","turn":{"id":"turn-2","status":"inProgress","items":[]}}}`)
	r.feed(question("turn-3"))
	if len(sink.requests) != 0 {
		t.Errorf("a question of no running turn was raised: %v", sink.requests)
	}
	if len(sink.events) != 2 || sink.events[0].Kind != model.EventAssistant || sink.events[1].Kind != model.EventAssistant {
		t.Errorf("the messages are recorded as %v", summary(sink.events))
	}
	r.feed(question("turn-2"))
	if strings.Join(sink.requests, ",") != "question:svc-1" {
		t.Errorf("the running turn's question: %v", sink.requests)
	}
	w.quiet(t)
}

// a turn starts when codex names it, by turn/started or by its reply to
// turn/start, whichever comes first, and once: the other names it again and
// starts nothing. an item of a turn that has ended starts none.
func TestATurnStartsWhenCodexNamesItOnce(t *testing.T) {
	for _, first := range []string{"turn/started", "reply"} {
		t.Run(first, func(t *testing.T) {
			r, sink, w := testRuntime()
			result := make(chan error, 1)
			go func() { result <- r.Prompt("svc-uuid", "Reply with ALPHA") }()
			start := w.sent(t)
			started := `{"method":"turn/started","params":{"threadId":"thread-1","turn":{"id":"turn-1","status":"inProgress","items":[]}}}`
			reply := fmt.Sprintf(`{"id":%v,"result":{"turn":{"id":"turn-1","status":"inProgress","items":[]}}}`, start["id"])
			if first == "reply" {
				r.feed(reply)
				if err := <-result; err != nil {
					t.Fatal(err)
				}
				if len(sink.turnStarts) != 1 {
					t.Fatalf("after the reply, turn starts declared at %v, want one", sink.turnStarts)
				}
				r.feed(started)
			} else {
				r.feed(started)
				if len(sink.turnStarts) != 1 {
					t.Fatalf("after turn/started, turn starts declared at %v, want one", sink.turnStarts)
				}
				r.feed(reply)
				if err := <-result; err != nil {
					t.Fatal(err)
				}
			}
			r.feed(`{"method":"item/completed","params":{"threadId":"thread-1","turnId":"turn-1","item":{"type":"agentMessage","id":"a-1","text":"ALPHA"}}}`)
			r.feed(`{"method":"turn/completed","params":{"threadId":"thread-1","turn":{"id":"turn-1","status":"completed","items":[]}}}`)
			r.feed(`{"method":"item/completed","params":{"threadId":"thread-1","turnId":"turn-1","item":{"type":"agentMessage","id":"a-2","text":"late"}}}`)
			if len(sink.turnStarts) != 1 || sink.turnStarts[0] != 0 {
				t.Errorf("turn starts declared at %v, want one, before any event", sink.turnStarts)
			}
			// a turn codex begins by itself, named by turn/started alone,
			// starts too.
			r.feed(`{"method":"turn/started","params":{"threadId":"thread-1","turn":{"id":"turn-2","status":"inProgress","items":[]}}}`)
			if len(sink.turnStarts) != 2 {
				t.Errorf("a turn codex began by itself: turn starts declared at %v", sink.turnStarts)
			}
		})
	}
}

// a turn that fails before the reply to its turn/start arrives is not marked
// running by that reply, and a question it asked raises no request.
func TestALateReplyNeverMarksAnEndedTurnRunning(t *testing.T) {
	r, sink, w := testRuntime()
	result := make(chan error, 1)
	go func() { result <- r.Prompt("svc-uuid", "Reply with ALPHA") }()
	start := w.sent(t)
	if start["method"] != "turn/start" {
		t.Fatalf("sent %v", start["method"])
	}
	r.feed(`{"method":"turn/completed","params":{"threadId":"thread-1","turn":{"id":"turn-7","status":"failed","items":[]}}}`)
	r.feed(fmt.Sprintf(`{"id":%v,"result":{"turn":{"id":"turn-7","status":"inProgress","items":[]}}}`, start["id"]))
	if err := <-result; err != nil {
		t.Fatal(err)
	}
	r.mu.Lock()
	active := r.activeTurn
	r.mu.Unlock()
	if active != "" {
		t.Errorf("the failed turn is marked running: %q", active)
	}
	if len(sink.turnStarts) != 0 {
		t.Errorf("the reply to an ended turn declared a start, at %v", sink.turnStarts)
	}
	r.feed(`{"method":"item/completed","params":{"threadId":"thread-1","turnId":"turn-7","item":{"type":"agentMessage","id":"a-1","text":"Which color?","questions":[{"title":"Which color do you prefer?","options":["Red","Blue"]}]}}}`)
	if len(sink.requests) != 0 {
		t.Errorf("a question of the failed turn was raised: %v", sink.requests)
	}
}

// a steer written while its turn runs settles from codex's reply, whatever
// the turn does meanwhile: the turn's end aborts nothing, a reply that
// succeeds delivers the answer, and one that fails refuses it. turn/steer is
// the last thing sent either way, so nothing falls back to starting a turn.
func TestASteerSettlesFromItsReplyAfterTheTurnEnds(t *testing.T) {
	for _, reply := range []string{"success", "failure"} {
		t.Run(reply, func(t *testing.T) {
			r, sink, w := testRuntime()
			askQuestion(t, r)
			result := make(chan error, 1)
			go func() {
				result <- r.Answer("svc-1", harness.Answer{Answers: map[string]string{"Which color do you prefer?": "Blue"}})
			}()
			steer := w.sent(t)
			if steer["method"] != "turn/steer" {
				t.Fatalf("sent %v", steer["method"])
			}
			r.feed(`{"method":"turn/completed","params":{"threadId":"thread-1","turn":{"id":"turn-1","status":"completed","items":[]}}}`)
			if len(sink.resolved) != 0 {
				t.Errorf("the turn's end aborted %v", sink.resolved)
			}
			want := error(nil)
			if reply == "success" {
				r.feed(fmt.Sprintf(`{"id":%v,"result":{"turnId":"turn-1"}}`, steer["id"]))
			} else {
				r.feed(fmt.Sprintf(`{"id":%v,"error":{"code":-32600,"message":"no active turn"}}`, steer["id"]))
				want = harness.ErrNotPending
			}
			select {
			case err := <-result:
				if !errors.Is(err, want) {
					t.Errorf("answer: %v, want %v", err, want)
				}
			case line := <-w.lines:
				t.Fatalf("after the steer, %s was sent", line)
			case <-time.After(2 * time.Second):
				t.Fatal("the answer did not return")
			}
			w.quiet(t)
		})
	}
}

// a steer written that codex never replies to, before the deadline or before
// the app-server goes, was neither taken nor refused as far as mega knows: the
// question is settled once, as unconfirmed, and before the exit is reported,
// so the exit's retirement cannot abort it as well; the operator is told
// codex did not confirm it, and nothing is sent again.
func TestASteerWithNoReplyIsUnconfirmed(t *testing.T) {
	for _, lost := range []string{"timeout", "connection"} {
		t.Run(lost, func(t *testing.T) {
			r, sink, w := testRuntime()
			if lost == "timeout" {
				r.steerTimeout = 50 * time.Millisecond
			}
			askQuestion(t, r)
			result := make(chan error, 1)
			go func() {
				result <- r.Answer("svc-1", harness.Answer{Answers: map[string]string{"Which color do you prefer?": "Blue"}})
			}()
			if steer := w.sent(t); steer["method"] != "turn/steer" {
				t.Fatalf("sent %v", steer["method"])
			}
			exited := make(chan struct{})
			if lost == "connection" {
				go func() {
					r.watch()
					close(exited)
				}()
				r.conn.end(nil)
			}
			select {
			case err := <-result:
				if !errors.Is(err, harness.ErrUnconfirmed) || err.Error() != "Codex did not confirm whether it received your answer" {
					t.Errorf("answer: %v", err)
				}
			case <-time.After(2 * time.Second):
				t.Fatal("the answer did not return")
			}
			if lost == "connection" {
				select {
				case <-exited:
				case <-time.After(2 * time.Second):
					t.Fatal("the exit was not reported")
				}
			}
			sink.mu.Lock()
			resolved, atExit, wasExited := slices.Clone(sink.resolved), sink.atExit, sink.exited
			sink.mu.Unlock()
			if strings.Join(resolved, ",") != "svc-1:unconfirmed" {
				t.Errorf("resolved = %v", resolved)
			}
			if wasExited && strings.Join(atExit, ",") != "svc-1:unconfirmed" {
				t.Errorf("when the exit was reported, resolved = %v", atExit)
			}
			if err := r.Answer("svc-1", harness.Answer{Answers: map[string]string{"Which color do you prefer?": "Red"}}); !errors.Is(err, harness.ErrNotPending) {
				t.Errorf("second answer: %v", err)
			}
			w.quiet(t)
		})
	}
}

// a steer codex replies to and then exits at once is settled by that reply,
// delivered or refused, before the exit is reported: the exit's retirement
// never settles it a second way, so what the operator is told and what is
// recorded agree.
func TestASteerItsReplySettledIsSettledBeforeTheExit(t *testing.T) {
	for _, reply := range []string{"success", "refusal"} {
		t.Run(reply, func(t *testing.T) {
			r, sink, w := testRuntime()
			askQuestion(t, r)
			result := make(chan error, 1)
			go func() {
				result <- r.Answer("svc-1", harness.Answer{Answers: map[string]string{"Which color do you prefer?": "Blue"}})
			}()
			steer := w.sent(t)
			if steer["method"] != "turn/steer" {
				t.Fatalf("sent %v", steer["method"])
			}
			want, outcome := error(nil), "svc-1:answered"
			if reply == "success" {
				r.feed(fmt.Sprintf(`{"id":%v,"result":{"turnId":"turn-1"}}`, steer["id"]))
			} else {
				r.feed(fmt.Sprintf(`{"id":%v,"error":{"code":-32600,"message":"no active turn"}}`, steer["id"]))
				want, outcome = harness.ErrNotPending, "svc-1:aborted"
			}
			exited := make(chan struct{})
			go func() {
				r.watch()
				close(exited)
			}()
			r.conn.end(nil)
			select {
			case <-exited:
			case <-time.After(2 * time.Second):
				t.Fatal("the exit was not reported")
			}
			if err := <-result; !errors.Is(err, want) {
				t.Errorf("answer: %v, want %v", err, want)
			}
			sink.mu.Lock()
			resolved, atExit := slices.Clone(sink.resolved), sink.atExit
			sink.mu.Unlock()
			if strings.Join(atExit, ",") != outcome || strings.Join(resolved, ",") != outcome {
				t.Errorf("resolved %v, and %v when the exit was reported; want %s", resolved, atExit, outcome)
			}
		})
	}
}

// the runtime's lock is held while turn/steer is written, and free while
// codex's reply is awaited, so a stop can begin before codex answers. this
// shows the lock at the write; that the fence is checked under the same hold
// is for review of Answer.
func TestTheSteerIsWrittenUnderTheRuntimesLock(t *testing.T) {
	r, _, w := testRuntime()
	askQuestion(t, r)
	var held atomic.Bool
	w.onWrite = func() {
		// the writer holding r.mu cannot take it again.
		if r.mu.TryLock() {
			r.mu.Unlock()
			return
		}
		held.Store(true)
	}
	result := make(chan error, 1)
	go func() {
		result <- r.Answer("svc-1", harness.Answer{Answers: map[string]string{"Which color do you prefer?": "Blue"}})
	}()
	steer := w.sent(t)
	if steer["method"] != "turn/steer" {
		t.Fatalf("sent %v", steer["method"])
	}
	if !held.Load() {
		t.Error("turn/steer was written without the runtime's lock")
	}
	stopped := make(chan struct{})
	go func() {
		r.Stopping()
		close(stopped)
	}()
	select {
	case <-stopped:
	case <-time.After(2 * time.Second):
		t.Fatal("a stop could not begin while the steer's reply was awaited")
	}
	r.feed(fmt.Sprintf(`{"id":%v,"result":{"turnId":"turn-1"}}`, steer["id"]))
	if err := <-result; err != nil {
		t.Errorf("answer: %v", err)
	}
}

// a resolution codex reports while an answer is on its way is not lost: one
// read before the answer is written settles the request, and the answer is
// not pending, as claude's shim says when it rejects one.
func TestAResolutionReadBeforeTheAnswerIsWrittenSettlesIt(t *testing.T) {
	r, sink, w := testRuntime()
	r.feed(`{"id":1,"method":"item/fileChange/requestApproval","params":{"threadId":"thread-1","turnId":"turn-1","itemId":"f-1","startedAtMs":1}}`)
	if len(sink.requests) != 1 {
		t.Fatalf("requests = %v", sink.requests)
	}
	early := false
	w.onWrite = func() {
		w.onWrite = nil
		done := make(chan struct{})
		go func() {
			r.feed(`{"method":"serverRequest/resolved","params":{"threadId":"thread-1","requestId":1}}`)
			close(done)
		}()
		select {
		case <-done:
			early = true
		case <-time.After(200 * time.Millisecond):
		}
	}
	err := r.Answer("svc-1", harness.Answer{Decision: harness.DecisionDeny})
	if early && err == nil && !slices.Contains(sink.resolved, "svc-1:aborted") {
		t.Error("a resolution read before the answer was written was lost, and the answer reported delivered")
	}
}
