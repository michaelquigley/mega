package events

import (
	"errors"
	"os"
	"os/signal"
	"path/filepath"
	"reflect"
	"syscall"
	"testing"
	"time"

	"github.com/michaelquigley/mega/internal/model"
)

func openLog(t *testing.T, path string) *Log {
	t.Helper()
	log, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { log.Close() })
	return log
}

func appendTexts(t *testing.T, log *Log, texts ...string) {
	t.Helper()
	for _, text := range texts {
		if err := log.Append(&model.Event{Kind: model.EventUser, Origin: model.OriginManaged, Text: text}); err != nil {
			t.Fatal(err)
		}
	}
}

func texts(evs []*model.Event) []string {
	out := []string{}
	for _, ev := range evs {
		out = append(out, ev.Text)
	}
	return out
}

func TestAppendAssignsSeqAndTime(t *testing.T) {
	log := openLog(t, filepath.Join(t.TempDir(), "conversations", "c1", "events.jsonl"))
	first := &model.Event{Kind: model.EventUser, Origin: model.OriginManaged, Text: "one"}
	if err := log.Append(first); err != nil {
		t.Fatal(err)
	}
	second := &model.Event{Kind: model.EventAssistant, Origin: model.OriginManaged, Text: "two"}
	if err := log.Append(second); err != nil {
		t.Fatal(err)
	}
	if first.Seq != 1 || second.Seq != 2 || log.LastSeq() != 2 {
		t.Errorf("seqs = %d %d %d", first.Seq, second.Seq, log.LastSeq())
	}
	if first.At.IsZero() {
		t.Error("at was not stamped")
	}
}

func TestSinceAndTail(t *testing.T) {
	log := openLog(t, filepath.Join(t.TempDir(), "events.jsonl"))
	appendTexts(t, log, "a", "b", "c", "d")

	for _, tc := range []struct {
		name string
		got  func() ([]*model.Event, error)
		want []string
	}{
		{"since zero", func() ([]*model.Event, error) { return log.Since(0) }, []string{"a", "b", "c", "d"}},
		{"since two", func() ([]*model.Event, error) { return log.Since(2) }, []string{"c", "d"}},
		{"since end", func() ([]*model.Event, error) { return log.Since(4) }, []string{}},
		{"tail two", func() ([]*model.Event, error) { return log.Tail(2) }, []string{"c", "d"}},
		{"tail more than held", func() ([]*model.Event, error) { return log.Tail(9) }, []string{"a", "b", "c", "d"}},
		{"tail zero", func() ([]*model.Event, error) { return log.Tail(0) }, []string{}},
	} {
		evs, err := tc.got()
		if err != nil {
			t.Fatalf("%s: %v", tc.name, err)
		}
		if got := texts(evs); !reflect.DeepEqual(got, tc.want) {
			t.Errorf("%s: got %v, want %v", tc.name, got, tc.want)
		}
	}
}

func TestAppendAfterReadStaysAtEnd(t *testing.T) {
	log := openLog(t, filepath.Join(t.TempDir(), "events.jsonl"))
	appendTexts(t, log, "a")
	if _, err := log.Since(0); err != nil {
		t.Fatal(err)
	}
	appendTexts(t, log, "b")
	evs, err := log.Since(0)
	if err != nil {
		t.Fatal(err)
	}
	if got := texts(evs); !reflect.DeepEqual(got, []string{"a", "b"}) {
		t.Errorf("got %v", got)
	}
}

func TestReopenContinuesSeq(t *testing.T) {
	path := filepath.Join(t.TempDir(), "events.jsonl")
	log, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	appendTexts(t, log, "a", "b")
	log.Close()

	again := openLog(t, path)
	if again.LastSeq() != 2 {
		t.Fatalf("last seq = %d", again.LastSeq())
	}
	appendTexts(t, again, "c")
	if again.LastSeq() != 3 {
		t.Errorf("last seq = %d", again.LastSeq())
	}
}

func TestFailedAppendRollsBack(t *testing.T) {
	path := filepath.Join(t.TempDir(), "events.jsonl")
	log := openLog(t, path)
	appendTexts(t, log, "a")
	sizeBefore := log.size

	// a sync failure cannot be produced on a real descriptor, but it shares the
	// write's rollback path. a file size limit just past the confirmed length
	// makes the write land partly and then fail, while the truncate still works.
	signal.Ignore(syscall.SIGXFSZ)
	t.Cleanup(func() { signal.Reset(syscall.SIGXFSZ) })
	var limit syscall.Rlimit
	if err := syscall.Getrlimit(syscall.RLIMIT_FSIZE, &limit); err != nil {
		t.Fatal(err)
	}
	lowered := syscall.Rlimit{Cur: uint64(sizeBefore + 4), Max: limit.Max}
	if err := syscall.Setrlimit(syscall.RLIMIT_FSIZE, &lowered); err != nil {
		t.Skipf("cannot lower the file size limit: %v", err)
	}
	ev := &model.Event{Kind: model.EventUser, Origin: model.OriginManaged, Text: "lost"}
	err := log.Append(ev)
	if restoreErr := syscall.Setrlimit(syscall.RLIMIT_FSIZE, &limit); restoreErr != nil {
		t.Fatal(restoreErr)
	}
	if err == nil || errors.Is(err, ErrUncertain) {
		t.Fatalf("expected an ordinary write failure, got %v", err)
	}

	if log.LastSeq() != 1 || log.size != sizeBefore || ev.Seq != 0 {
		t.Fatalf("failed append advanced the log: last=%d size=%d seq=%d", log.LastSeq(), log.size, ev.Seq)
	}
	if info, err := os.Stat(path); err != nil || info.Size() != sizeBefore {
		t.Fatalf("partial write was not truncated back: %v %v", info, err)
	}
	appendTexts(t, log, "b")
	evs, err := log.Since(0)
	if err != nil {
		t.Fatal(err)
	}
	if got := texts(evs); !reflect.DeepEqual(got, []string{"a", "b"}) || evs[1].Seq != 2 {
		t.Errorf("got %v, second seq %d", got, evs[1].Seq)
	}
}

func TestFailedRollbackIsSticky(t *testing.T) {
	path := filepath.Join(t.TempDir(), "events.jsonl")
	log := openLog(t, path)
	appendTexts(t, log, "a")

	// stand in for a write that landed before failing: a complete line past the
	// confirmed length, which no read may serve.
	other, err := os.OpenFile(path, os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := other.WriteString(`{"seq":2,"kind":"user","origin":"managed","text":"unconfirmed"}` + "\n"); err != nil {
		t.Fatal(err)
	}
	other.Close()

	// a read-only descriptor fails both the write and the truncate.
	writable := log.file
	readOnly, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	log.file = readOnly
	err = log.Append(&model.Event{Kind: model.EventUser, Origin: model.OriginManaged, Text: "lost"})
	log.file = writable
	readOnly.Close()
	if !errors.Is(err, ErrUncertain) {
		t.Fatalf("expected ErrUncertain, got %v", err)
	}

	evs, err := log.Since(0)
	if err != nil {
		t.Fatal(err)
	}
	if got := texts(evs); !reflect.DeepEqual(got, []string{"a"}) {
		t.Errorf("reads served past the confirmed length: %v", got)
	}
	if tail, err := log.Tail(5); err != nil || !reflect.DeepEqual(texts(tail), []string{"a"}) {
		t.Errorf("tail = %v, %v", texts(tail), err)
	}

	// a working descriptor does not lift the refusal; only a reopen does.
	if err := log.Append(&model.Event{Kind: model.EventUser, Text: "b"}); !errors.Is(err, ErrUncertain) {
		t.Fatalf("later append was not refused: %v", err)
	}
	if log.LastSeq() != 1 {
		t.Errorf("last seq = %d", log.LastSeq())
	}
}

func TestOpenCreatesNestedDirectories(t *testing.T) {
	path := filepath.Join(t.TempDir(), "conversations", "c1", "events.jsonl")
	log := openLog(t, path)
	appendTexts(t, log, "a")
	if _, err := os.Stat(path); err != nil {
		t.Fatal(err)
	}
}

func TestTornTailIsDroppedAndAppendsStayReadable(t *testing.T) {
	path := filepath.Join(t.TempDir(), "events.jsonl")
	log, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	appendTexts(t, log, "a")
	log.Close()

	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.WriteString(`{"seq":2,"kind":"us`); err != nil {
		t.Fatal(err)
	}
	f.Close()

	again := openLog(t, path)
	if again.LastSeq() != 1 {
		t.Fatalf("last seq = %d", again.LastSeq())
	}
	appendTexts(t, again, "b")
	evs, err := again.Since(0)
	if err != nil {
		t.Fatal(err)
	}
	if got := texts(evs); !reflect.DeepEqual(got, []string{"a", "b"}) {
		t.Errorf("got %v", got)
	}
}

func TestCorruptCompleteLineIsAnErrorAndLeftInPlace(t *testing.T) {
	path := filepath.Join(t.TempDir(), "events.jsonl")
	body := []byte("{\"seq\":1,\"kind\":\"user\"}\nnot json\n{\"seq\":3,\"kind\":\"user\"}\n")
	if err := os.WriteFile(path, body, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Open(path); err == nil {
		t.Fatal("expected an error")
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(after) != string(body) {
		t.Error("open altered a file it could not read")
	}
}

func TestEventFieldsRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "events.jsonl")
	log := openLog(t, path)
	at := time.Date(2026, 11, 6, 12, 0, 0, 0, time.UTC)
	raw := map[string]any{"type": "assistant", "nested": []any{"x", 1.0, nil}}
	ev := &model.Event{
		At: at, Kind: model.EventRequest, Origin: model.OriginService, Text: "title", Raw: raw,
		Req: &model.Request{
			ID: "r1", Kind: model.RequestPermission, Outcome: model.OutcomeLost,
			Payload: map[string]any{"tool": "Bash", "input": map[string]any{"command": "ls"}},
		},
	}
	if err := log.Append(ev); err != nil {
		t.Fatal(err)
	}
	evs, err := log.Since(0)
	if err != nil {
		t.Fatal(err)
	}
	if len(evs) != 1 {
		t.Fatalf("got %d events", len(evs))
	}
	got := evs[0]
	if !got.At.Equal(at) || got.Kind != ev.Kind || got.Origin != ev.Origin || got.Text != ev.Text {
		t.Errorf("event = %+v", got)
	}
	if !reflect.DeepEqual(got.Raw, raw) {
		t.Errorf("raw = %#v", got.Raw)
	}
	if got.Req == nil || got.Req.ID != "r1" || got.Req.Outcome != model.OutcomeLost {
		t.Fatalf("req = %+v", got.Req)
	}
	if !reflect.DeepEqual(got.Req.Payload, ev.Req.Payload) {
		t.Errorf("payload = %#v", got.Req.Payload)
	}
}

func TestLargeEventsAreReadable(t *testing.T) {
	log := openLog(t, filepath.Join(t.TempDir(), "events.jsonl"))
	big := make([]byte, 5<<20)
	for i := range big {
		big[i] = 'x'
	}
	appendTexts(t, log, string(big), "after")
	evs, err := log.Since(0)
	if err != nil {
		t.Fatal(err)
	}
	if len(evs) != 2 || len(evs[0].Text) != len(big) || evs[1].Text != "after" {
		t.Errorf("got %d events", len(evs))
	}
}

func TestRemoveDeletesTheLogAndItsDirectory(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "conversations", "c1", "events.jsonl")
	log, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	appendTexts(t, log, "one")
	log.Close()
	keep := filepath.Join(root, "conversations", "c2", "events.jsonl")
	openLog(t, keep)

	if err := Remove(path); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Dir(path)); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("the directory survived: %v", err)
	}
	if _, err := os.Stat(keep); err != nil {
		t.Errorf("another log went with it: %v", err)
	}
	if err := Remove(path); err != nil {
		t.Errorf("removing it again: %v", err)
	}
}
