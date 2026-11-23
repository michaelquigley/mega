package claude

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/michaelquigley/mega/internal/harness"
	"github.com/michaelquigley/mega/internal/model"
)

type recordingSink struct {
	events   []*model.Event
	deltas   []string
	requests []string
	payloads []any
	resolved []string
	// background holds every list reported, in order; mcp every MCP list.
	background [][]model.BackgroundJob
	mcp        [][]model.MCPServer
	next       int
}

func (s *recordingSink) MCPServers(servers []model.MCPServer) {
	s.mcp = append(s.mcp, servers)
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

func TestPromptRefusesAKnownWrongSessionBeforeWriting(t *testing.T) {
	r, _, stdin := testRuntime()
	if !r.sameSession(r.NativeID()) {
		t.Fatal("the matching session was refused")
	}
	if err := r.Prompt("matching-id", "matching prompt"); err != nil || !strings.Contains(stdin.String(), "matching prompt") {
		t.Fatalf("matching prompt did not reach the shim: %v", err)
	}

	stdin.Reset()
	if r.sameSession("another-session") {
		t.Fatal("the reported session should have tripped the latch")
	}
	if err := r.Prompt("disputed-id", "disputed prompt"); !errors.Is(err, harness.ErrPromptRefused) {
		t.Errorf("mismatched prompt returned %v", err)
	}
	if stdin.Len() != 0 {
		t.Error("the mismatched prompt reached the shim")
	}
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

	if err := answerWith(r, "svc-1", harness.Answer{Decision: harness.DecisionDeny}, `{"type":"answer_accepted","id":"sdk-1"}`); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stdin.String(), `"id":"sdk-1"`) || strings.Contains(stdin.String(), "svc-1") {
		t.Errorf("answer frame did not carry the sdk id alone: %s", stdin.String())
	}
	if err := r.Answer("svc-1", harness.Answer{Decision: harness.DecisionDeny}); !errors.Is(err, harness.ErrNotPending) {
		t.Errorf("second answer: %v", err)
	}

	r.handle([]byte(`{"type":"request_resolved","id":"sdk-2","outcome":"aborted"}`))
	if strings.Join(sink.resolved, ",") != "svc-2:aborted" {
		t.Errorf("resolved = %v", sink.resolved)
	}
	if err := r.Answer("svc-2", harness.Answer{Decision: harness.DecisionDismiss}); !errors.Is(err, harness.ErrNotPending) {
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
	err := answerWith(r, "svc-1", harness.Answer{Decision: harness.DecisionDeny}, `{"type":"answer_rejected","id":"sdk-1"}`)
	if !errors.Is(err, harness.ErrNotPending) {
		t.Errorf("rejected answer: %v", err)
	}
	r.handle([]byte(`{"type":"request_resolved","id":"sdk-1","outcome":"aborted"}`))
	if len(sink.resolved) != 0 {
		t.Errorf("the late abort notice was reported again: %v", sink.resolved)
	}
}

// once a stop is fenced, an answer that would let work go on never reaches
// the shim, even one accepted before the stop: the interrupt that follows
// aborts the request. a denial still goes.
func TestNoAnswerLetsWorkGoOnAfterTheStopIsFenced(t *testing.T) {
	r, sink, stdin := testRuntime()
	r.handle([]byte(`{"type":"request","id":"sdk-1","kind":"permission","tool":"Bash","input":{"command":"echo fenced"}}`))
	// the request the allow is tried on is the one raised, looked at first.
	if b, _ := json.Marshal(sink.payloads); len(sink.payloads) != 1 || !strings.Contains(string(b), `"command":"echo fenced"`) {
		t.Fatalf("the request is not the one meant: %s", b)
	}
	r.Stopping()
	result := make(chan error, 1)
	go func() { result <- r.Answer("svc-1", harness.Answer{Decision: harness.DecisionAllow}) }()
	select {
	case err := <-result:
		if !errors.Is(err, harness.ErrNotPending) {
			t.Errorf("allow after the fence: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("an allow after the fence went to the shim and waits on it")
	}
	if strings.Contains(stdin.String(), `"type":"answer"`) {
		t.Errorf("the shim was sent %s", stdin.String())
	}
}

// every frame that names a session is checked against the one the runtime is
// bound to, not only the first: a later frame from another session is
// reported once, so the service stops the runtime, and is never relayed.
func TestEveryFramesSessionIsChecked(t *testing.T) {
	r, sink, _ := testRuntime()
	r.handle([]byte(`{"type":"event","msg":{"type":"system","subtype":"init","session_id":"edfe4ade-9f59-4db3-89bc-75fcc8b52c44","model":"m"}}`))
	r.handle([]byte(`{"type":"event","msg":{"type":"assistant","session_id":"another-session","uuid":"x-1","parent_tool_use_id":null,"message":{"role":"assistant","content":[{"type":"text","text":"from another session"}]}}}`))
	r.handle([]byte(`{"type":"event","msg":{"type":"assistant","session_id":"another-session","uuid":"x-2","parent_tool_use_id":null,"message":{"role":"assistant","content":[{"type":"text","text":"and more"}]}}}`))
	reports := 0
	for _, ev := range sink.events {
		if ev.Kind == model.EventAssistant {
			t.Errorf("a frame from another session was relayed: %q", ev.Text)
		}
		if ev.Kind == model.EventRuntime && ev.Status == model.StatusStarted && ev.Facts != nil && ev.Facts.Session == "another-session" {
			reports++
		}
	}
	if reports != 1 {
		t.Errorf("the other session was reported %d times", reports)
	}
}

// shimExiting runs a stand-in for the shim, in its own process group as the
// shim is, that leaves a child of its own running, and reads it to its exit.
func shimExiting(t *testing.T) (*runtime, *exitSink, *exec.Cmd) {
	t.Helper()
	cmd := exec.Command("sh", "-c", "sleep 30 >/dev/null 2>&1 & echo $! ; exit 0")
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL) })
	r, _, _ := testRuntime()
	sink := &exitSink{exited: make(chan error, 1)}
	r.sink, r.cmd = sink, cmd
	go r.read(stdout)
	return r, sink, cmd
}

// the shim's exit is checked for the processes it leaves: a child still
// running is named, never a clean stop.
func TestAShimThatLeavesAChildRunningSaysSo(t *testing.T) {
	was := survivorGrace
	survivorGrace = 300 * time.Millisecond
	t.Cleanup(func() { survivorGrace = was })
	_, sink, _ := shimExiting(t)
	select {
	case err := <-sink.exited:
		if !errors.Is(err, harness.ErrLeftRunning) {
			t.Errorf("exit = %v", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("no exit")
	}
}

// where the processes cannot be listed, the exit says it could not check.
func TestAShimExitThatCouldNotLookSaysSo(t *testing.T) {
	was := harness.ProcRoot
	harness.ProcRoot = filepath.Join(t.TempDir(), "no-proc")
	t.Cleanup(func() { harness.ProcRoot = was })
	_, sink, _ := shimExiting(t)
	select {
	case err := <-sink.exited:
		if !errors.Is(err, harness.ErrUnchecked) {
			t.Errorf("exit = %v", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("no exit")
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
	go func() { result <- r.Answer("svc-1", harness.Answer{Decision: harness.DecisionDeny}) }()
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

// an interrupted turn, read back through the shim's messages command: claude
// code 2.1.319 wrote "No response requested." to the transcript as an
// assistant message from the model "<synthetic>", and the shim adds the
// transcript's isApiErrorMessage, false (recorded 2026-11-15 from the
// claude-rt1 fixture, usage and diagnostics dropped). it is claude's
// bookkeeping, a native event, and never a reply; the messages around it map
// as before.
func TestInterruptedTurnBookkeepingIsNative(t *testing.T) {
	data, err := os.ReadFile("testdata/interrupted-history.jsonl")
	if err != nil {
		t.Fatal(err)
	}
	var evs []*model.Event
	for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		r, sink, _ := testRuntime()
		r.nativeID, r.started = "bdfb220d-d5f1-4181-b9e1-fae55e0d8373", true // the recording's own session
		r.handle([]byte(`{"type":"event","msg":` + line + `}`))
		evs = append(evs, sink.events...)
	}
	var synthetic *model.Event
	for _, ev := range evs {
		if ev.NativeID == "b54150b2-5f5f-4d74-ab50-cc94c577c0ed" {
			synthetic = ev
		}
		if ev.Kind == model.EventAssistant && strings.Contains(ev.Text, "No response requested") {
			t.Errorf("the bookkeeping is a reply: %+v", ev)
		}
	}
	if synthetic == nil || synthetic.Kind != model.EventNative || synthetic.Text != "assistant/synthetic" {
		t.Fatalf("the bookkeeping maps to %+v", synthetic)
	}
	replies := 0
	for _, ev := range evs {
		if ev.Kind == model.EventAssistant {
			replies++
		}
	}
	if replies != 1 || evs[len(evs)-1].Kind != model.EventAssistant || evs[len(evs)-1].Text != "BETA-START-9e1f" {
		t.Errorf("the reply after it: %d replies, last %+v", replies, evs[len(evs)-1])
	}
}

// an api error is also written by no model, and stays a reply the operator
// reads: live, where it carries its error; read back, where the shim adds
// the transcript's flags; and whenever the transcript could not say, since a
// message of unknown kind is shown, never hidden.
func TestAnAPIErrorStaysAReply(t *testing.T) {
	const body = `"type":"assistant","uuid":"c0ffee00-0000-4000-8000-0000000000e1","session_id":"s","message":{"model":"<synthetic>","role":"assistant","content":[{"type":"text","text":"Login reached its time limit"}]},"parent_tool_use_id":null`
	for name, msg := range map[string]string{
		"live":            `{` + body + `,"error":"authentication_failed"}`,
		"read back":       `{` + body + `,"isApiErrorMessage":true,"error":"authentication_failed"}`,
		"flag only":       `{` + body + `,"isApiErrorMessage":true}`,
		"transcript gone": `{` + body + `}`,
	} {
		r, sink, _ := testRuntime()
		r.nativeID, r.started = "s", true
		r.handle([]byte(`{"type":"event","msg":` + msg + `}`))
		if len(sink.events) != 1 || sink.events[0].Kind != model.EventAssistant {
			t.Errorf("%s: events = %+v", name, sink.events)
		}
	}
}

// once a frame has named another session, nothing more the shim reports is
// passed on, a request as well as an event.
func TestNoRequestIsRaisedAfterAnotherSession(t *testing.T) {
	r, sink, _ := testRuntime()
	r.nativeID, r.started = "s", true
	r.handle([]byte(`{"type":"event","msg":{"type":"system","subtype":"status","session_id":"another"}}`))
	r.handle([]byte(`{"type":"request","id":"sdk-1","kind":"permission","tool":"Bash","input":{"command":"echo"}}`))
	if len(sink.requests) != 0 {
		t.Errorf("a request was raised after another session was reported: %v", sink.requests)
	}
}
