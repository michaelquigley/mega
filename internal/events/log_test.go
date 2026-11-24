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

	"github.com/michaelquigley/mega/internal/durable"
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

// a write that lands partly and fails is returned, and nothing of it is
// confirmed: the seq stays, no read serves it, and a reopen drops what
// reached the disk as a torn tail.
func TestAFailedWriteIsReturnedAndNotConfirmed(t *testing.T) {
	path := filepath.Join(t.TempDir(), "events.jsonl")
	log := openLog(t, path)
	appendTexts(t, log, "a")
	sizeBefore := log.size

	// a file size limit just past the confirmed length makes the write land
	// partly and then fail.
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
	if err == nil {
		t.Fatal("a failed write was not returned")
	}
	if log.LastSeq() != 1 || log.size != sizeBefore || ev.Seq != 0 {
		t.Fatalf("a failed append advanced the log: last=%d size=%d seq=%d", log.LastSeq(), log.size, ev.Seq)
	}
	if evs, err := log.Since(0); err != nil || !reflect.DeepEqual(texts(evs), []string{"a"}) {
		t.Errorf("reads = %v, %v", texts(evs), err)
	}
	log.Close()
	if evs, err := openLog(t, path).Since(0); err != nil || !reflect.DeepEqual(texts(evs), []string{"a"}) {
		t.Errorf("after a reopen = %v, %v", texts(evs), err)
	}
}

// a sync that fails is returned as a write's failure is, and the event is not
// confirmed, though its line reached the file.
func TestAFailedSyncIsReturnedAndNotConfirmed(t *testing.T) {
	log := openLog(t, filepath.Join(t.TempDir(), "events.jsonl"))
	appendTexts(t, log, "a")
	syncFile = func(*os.File) error { return errors.New("injected sync failure") }
	t.Cleanup(func() { syncFile = (*os.File).Sync })
	ev := &model.Event{Kind: model.EventUser, Origin: model.OriginManaged, Text: "unconfirmed"}
	if err := log.Append(ev); err == nil {
		t.Fatal("a failed sync was not returned")
	}
	if log.LastSeq() != 1 || ev.Seq != 0 {
		t.Fatalf("a failed sync advanced the log: last=%d seq=%d", log.LastSeq(), ev.Seq)
	}
	if evs, err := log.Since(0); err != nil || !reflect.DeepEqual(texts(evs), []string{"a"}) {
		t.Errorf("reads served past the confirmed length: %v, %v", texts(evs), err)
	}
}

// opening a log that does not exist writes nothing; its first append
// creates it, nested directories and all, and once it is closed nothing
// creates it again.
func TestTheFirstAppendCreatesTheLog(t *testing.T) {
	path := filepath.Join(t.TempDir(), "conversations", "c1", "events.jsonl")
	log := openLog(t, path)
	if _, err := os.Stat(filepath.Dir(path)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("opening wrote its directory: %v", err)
	}
	if evs, err := log.Since(0); err != nil || len(evs) != 0 || log.LastSeq() != 0 {
		t.Errorf("an absent log reads %v, %v, last %d", texts(evs), err, log.LastSeq())
	}
	appendTexts(t, log, "a")
	if _, err := os.Stat(path); err != nil {
		t.Fatal(err)
	}

	unused := filepath.Join(filepath.Dir(path), "..", "c2", "events.jsonl")
	closed := openLog(t, unused)
	closed.Close()
	if err := closed.Append(&model.Event{Kind: model.EventUser, Text: "late"}); err == nil {
		t.Error("a closed log took an append")
	}
	if _, err := os.Stat(filepath.Dir(unused)); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("a closed log was created: %v", err)
	}
}

// the first event syncs the directory naming the log, whether the append
// created the file or found it empty, as a crash after its creation, or a
// failed sync of its directory, leaves it; later events sync no directory.
func TestTheFirstEventSyncsTheLogsDirectory(t *testing.T) {
	var synced []string
	syncDir = func(dir string) error { synced = append(synced, dir); return nil }
	t.Cleanup(func() { syncDir = durable.SyncDir })

	created := filepath.Join(t.TempDir(), "c1", "events.jsonl")
	log := openLog(t, created)
	appendTexts(t, log, "a", "b")
	if !reflect.DeepEqual(synced, []string{filepath.Dir(created)}) {
		t.Errorf("a created log synced %v", synced)
	}

	synced = nil
	empty := filepath.Join(t.TempDir(), "events.jsonl")
	if err := os.WriteFile(empty, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	log = openLog(t, empty)
	appendTexts(t, log, "a", "b")
	if !reflect.DeepEqual(synced, []string{filepath.Dir(empty)}) {
		t.Errorf("an empty log found at open synced %v", synced)
	}
}

// a directory sync that fails confirms nothing and writes nothing, so the
// next open finds the log still empty and its first event syncs again.
func TestAFailedDirectorySyncConfirmsNothing(t *testing.T) {
	syncDir = func(string) error { return errors.New("injected directory sync failure") }
	t.Cleanup(func() { syncDir = durable.SyncDir })
	path := filepath.Join(t.TempDir(), "events.jsonl")
	log := openLog(t, path)
	ev := &model.Event{Kind: model.EventUser, Origin: model.OriginManaged, Text: "unconfirmed"}
	if err := log.Append(ev); err == nil {
		t.Fatal("a failed directory sync was not returned")
	}
	if log.LastSeq() != 0 || ev.Seq != 0 {
		t.Errorf("a failed directory sync advanced the log: last=%d seq=%d", log.LastSeq(), ev.Seq)
	}
	if info, err := os.Stat(path); err != nil || info.Size() != 0 {
		t.Errorf("a failed directory sync left the file %v, %v; want it empty", info, err)
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
	appendTexts(t, openLog(t, keep), "two")

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
