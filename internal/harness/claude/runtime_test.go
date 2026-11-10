package claude

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"git.hq.quigley.com/research/roundhouse/internal/harness"
	"git.hq.quigley.com/research/roundhouse/internal/model"
)

type recordingSink struct {
	events   []*model.Event
	deltas   []string
	requests []string
	payloads []any
	resolved []string
	// background holds every list reported, in order.
	background [][]model.BackgroundJob
	next       int
}

func (s *recordingSink) Background(jobs []model.BackgroundJob) {
	s.background = append(s.background, jobs)
}

func (s *recordingSink) Event(ev *model.Event) { s.events = append(s.events, ev) }
func (s *recordingSink) Delta(text string)     { s.deltas = append(s.deltas, text) }
func (s *recordingSink) Request(kind string, payload any) string {
	s.payloads = append(s.payloads, payload)
	s.next++
	id := fmt.Sprintf("svc-%d", s.next)
	s.requests = append(s.requests, kind+":"+id)
	return id
}
func (s *recordingSink) Resolved(id, outcome string) { s.resolved = append(s.resolved, id+":"+outcome) }
func (s *recordingSink) Exited(error)                {}

type nopCloser struct{ bytes.Buffer }

func (*nopCloser) Close() error { return nil }

func testRuntime() (*runtime, *recordingSink, *nopCloser) {
	sink := &recordingSink{}
	stdin := &nopCloser{}
	return &runtime{
		nativeID: "edfe4ade-9f59-4db3-89bc-75fcc8b52c44",
		stdin:    stdin,
		sink:     sink,
		requests: make(map[string]string),
		byNative: make(map[string]string),
		answers:  make(map[string]chan error),
		modes:    make(map[string]*modeSwitch),
	}, sink, stdin
}

func feed(t *testing.T, r *runtime, path string) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	scanner := bufio.NewScanner(bytes.NewReader(data))
	for scanner.Scan() {
		r.handle([]byte(`{"type":"event","msg":` + scanner.Text() + `}`))
	}
}

func summary(evs []*model.Event) []string {
	var out []string
	for _, ev := range evs {
		out = append(out, strings.Join([]string{ev.Kind, ev.Status, ev.NativeID, ev.Tool, ev.ToolUseID, ev.Text}, "|"))
	}
	return out
}

func TestStreamMapping(t *testing.T) {
	r, sink, _ := testRuntime()
	feed(t, r, "testdata/stream.jsonl")

	got := summary(sink.events)
	want := []string{
		"runtime|started||||",
		"assistant||2a4f4293-f3c9-4ee1-9c31-513753192bf1|||ROUNDHOUSE-MARKER-7f3a ALPHA-1a2b",
		"turn_end|success|9e7b2c11-0000-4000-8000-000000000002|||success",
		"native||49784537-c9f5-4711-84a5-49761d06da0e|||rate_limit_event",
		"tool_use||ba2ac7b2-2fde-47dc-8649-0d6a88250ef9|Bash|toolu_01Qt9227Nn1bc9AMNxX4aQCK|Bash cat fixture.txt",
		"tool_result||7491afd3-5043-402c-a735-1286234a7c66||toolu_01Qt9227Nn1bc9AMNxX4aQCK|token: FIXTURE-TOKEN-91c2",
		"turn_end|error|9e7b2c11-0000-4000-8000-000000000003|||error_during_execution",
	}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("events:\n%s\nwant:\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
	facts := sink.events[0].Facts
	if facts == nil || facts.Session != r.nativeID || facts.Model != "claude-fable-5-1" || facts.Version != "2.1.318" || facts.PermissionMode != "default" {
		t.Errorf("facts = %+v", facts)
	}
	for _, ev := range sink.events {
		if ev.Origin != model.OriginManaged || ev.Raw == nil {
			t.Errorf("event %s: origin %q, raw %v", ev.Kind, ev.Origin, ev.Raw != nil)
		}
	}
	if strings.Join(sink.deltas, "") != "ROUNDHOUSE-" {
		t.Errorf("deltas = %v", sink.deltas)
	}
}

func TestSecondInitIsNative(t *testing.T) {
	r, sink, _ := testRuntime()
	feed(t, r, "testdata/stream.jsonl")
	feed(t, r, "testdata/stream.jsonl")
	if sink.events[len(sink.events)/2].Kind != model.EventNative {
		t.Errorf("a later init was reported as a runtime start: %+v", sink.events[len(sink.events)/2])
	}
}

func TestInterruptedTurnEnd(t *testing.T) {
	r, sink, _ := testRuntime()
	if err := r.Interrupt(); err != nil {
		t.Fatal(err)
	}
	r.handle([]byte(`{"type":"event","msg":{"type":"result","subtype":"error_during_execution","is_error":true,"uuid":"x"}}`))
	r.handle([]byte(`{"type":"event","msg":{"type":"result","subtype":"success","uuid":"y"}}`))
	if sink.events[0].Status != model.TurnInterrupted || sink.events[1].Status != model.TurnSuccess {
		t.Errorf("statuses = %q, %q", sink.events[0].Status, sink.events[1].Status)
	}
}

func TestHistoryMatchesLiveIdentity(t *testing.T) {
	data, err := os.ReadFile("testdata/history.jsonl")
	if err != nil {
		t.Fatal(err)
	}
	var evs []*model.Event
	for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		r, sink, _ := testRuntime()
		r.started = true
		r.handle([]byte(`{"type":"event","msg":` + line + `}`))
		evs = append(evs, sink.events...)
	}
	got := summary(evs)
	want := []string{
		"user||ffc454d1-3530-4679-b50f-8b9b1908e175|||Reply with the project instruction marker followed by ALPHA-1a2b",
		"assistant||2a4f4293-f3c9-4ee1-9c31-513753192bf1|||ROUNDHOUSE-MARKER-7f3a ALPHA-1a2b",
		"user||b6e4daac-ad25-44f2-abb6-69e53648b50c|||Reply with TERMINAL-MARKER-e9d1",
		"native||c0ffee00-0000-4000-8000-000000000004|||assistant/thinking",
		"assistant||c0ffee00-0000-4000-8000-000000000004/1|||TERMINAL-MARKER-e9d1",
		"native||c0ffee00-0000-4000-8000-000000000005|||assistant",
	}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("events:\n%s\nwant:\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
}

func TestRequestsMapThroughServiceIDs(t *testing.T) {
	r, sink, stdin := testRuntime()
	r.handle([]byte(`{"type":"request","id":"sdk-1","kind":"permission","tool":"Bash","input":{"command":"sleep 3"},"toolUseId":"toolu_1","title":null}`))
	r.handle([]byte(`{"type":"request","id":"sdk-2","kind":"question","tool":"AskUserQuestion","input":{"questions":[{"question":"Which?"}]}}`))
	if strings.Join(sink.requests, ",") != "permission:svc-1,question:svc-2" {
		t.Fatalf("requests = %v", sink.requests)
	}

	if err := answerWith(r, "svc-1", harness.Answer{Decision: harness.DecisionAllow}, `{"type":"answer_accepted","id":"sdk-1"}`); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stdin.String(), `"id":"sdk-1"`) || strings.Contains(stdin.String(), "svc-1") {
		t.Errorf("answer frame did not carry the sdk id alone: %s", stdin.String())
	}
	if err := r.Answer("svc-1", harness.Answer{Decision: harness.DecisionAllow}); !errors.Is(err, harness.ErrNotPending) {
		t.Errorf("second answer: %v", err)
	}

	r.handle([]byte(`{"type":"request_resolved","id":"sdk-2","outcome":"aborted"}`))
	if strings.Join(sink.resolved, ",") != "svc-2:aborted" {
		t.Errorf("resolved = %v", sink.resolved)
	}
	if err := r.Answer("svc-2", harness.Answer{Answers: map[string]string{"Which?": "Blue"}}); !errors.Is(err, harness.ErrNotPending) {
		t.Errorf("answer after abort: %v", err)
	}
}

// answerWith answers on another goroutine and plays the shim's verdict.
func answerWith(r *runtime, id string, answer harness.Answer, verdict string) error {
	result := make(chan error, 1)
	go func() { result <- r.Answer(id, answer) }()
	for {
		r.mu.Lock()
		waiting := len(r.answers) > 0
		r.mu.Unlock()
		if waiting {
			break
		}
		time.Sleep(time.Millisecond)
	}
	r.handle([]byte(verdict))
	return <-result
}

// the sdk aborted the request after the adapter took the answer but before its
// abort notice was read: the shim rejects the answer and it is not pending.
func TestAnswerRejectedByShimIsNotPending(t *testing.T) {
	r, sink, _ := testRuntime()
	r.handle([]byte(`{"type":"request","id":"sdk-1","kind":"permission","tool":"Bash","input":{}}`))
	err := answerWith(r, "svc-1", harness.Answer{Decision: harness.DecisionAllow}, `{"type":"answer_rejected","id":"sdk-1"}`)
	if !errors.Is(err, harness.ErrNotPending) {
		t.Errorf("rejected answer: %v", err)
	}
	r.handle([]byte(`{"type":"request_resolved","id":"sdk-1","outcome":"aborted"}`))
	if len(sink.resolved) != 0 {
		t.Errorf("the late abort notice was reported again: %v", sink.resolved)
	}
}

func TestAnswerFailsWhenTheRuntimeEnds(t *testing.T) {
	r, _, _ := testRuntime()
	r.cmd = exec.Command("true")
	if err := r.cmd.Start(); err != nil {
		t.Fatal(err)
	}
	r.handle([]byte(`{"type":"request","id":"sdk-1","kind":"permission","tool":"Bash","input":{}}`))
	result := make(chan error, 1)
	go func() { result <- r.Answer("svc-1", harness.Answer{Decision: harness.DecisionAllow}) }()
	for {
		r.mu.Lock()
		waiting := len(r.answers) > 0
		r.mu.Unlock()
		if waiting {
			break
		}
		time.Sleep(time.Millisecond)
	}
	r.read(strings.NewReader(""))
	if err := <-result; !errors.Is(err, errExited) {
		t.Errorf("answer after exit: %v", err)
	}
}
