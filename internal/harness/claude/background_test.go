package claude

import (
	"strings"
	"testing"

	"git.hq.quigley.com/research/roundhouse/internal/model"
)

func descriptions(jobs []model.BackgroundJob) string {
	var out []string
	for _, job := range jobs {
		out = append(out, job.Description)
	}
	return strings.Join(out, ",")
}

// recorded from claude code 2.1.318 (probes/claude/background.mjs, two jobs):
// each background_tasks_changed snapshot replaces the list whole, down to the
// empty one; a job keeps the start it was first seen with; and the native
// frames are still recorded as they were.
func TestBackgroundSnapshotsReplaceTheList(t *testing.T) {
	r, sink, _ := testRuntime()
	feed(t, r, "testdata/background.jsonl")
	var got []string
	for _, jobs := range sink.background {
		got = append(got, "["+descriptions(jobs)+"]")
	}
	want := "[Wait 10 seconds then print marker A] [Wait 10 seconds then print marker A,Wait 20 seconds then print marker B] [Wait 20 seconds then print marker B] []"
	if strings.Join(got, " ") != want {
		t.Fatalf("lists reported:\n%s\nwant\n%s", strings.Join(got, " "), want)
	}
	first, second, third := sink.background[0], sink.background[1], sink.background[2]
	if first[0].ID != "bnvhpudbd" || first[0].Kind != "local_bash" || first[0].StartedAt.IsZero() {
		t.Errorf("first job = %+v", first[0])
	}
	if !second[0].StartedAt.Equal(first[0].StartedAt) || !third[0].StartedAt.Equal(second[1].StartedAt) {
		t.Errorf("a job's start moved between snapshots: %v %v / %v %v", first[0].StartedAt, second[0].StartedAt, second[1].StartedAt, third[0].StartedAt)
	}
	natives := 0
	for _, ev := range sink.events {
		if raw, _ := ev.Raw.(map[string]any); ev.Kind == model.EventNative && raw["subtype"] == "background_tasks_changed" {
			natives++
		}
	}
	if natives != 4 {
		t.Errorf("background_tasks_changed recorded as native %d times, want 4", natives)
	}
}

// a job that went and came back under the same id is new: its start is when
// it was seen again.
func TestBackgroundJobGoneThenBackStartsAfresh(t *testing.T) {
	r, sink, _ := testRuntime()
	snapshot := func(ids ...string) {
		var tasks []string
		for _, id := range ids {
			tasks = append(tasks, `{"task_id":"`+id+`","task_type":"local_agent","description":"`+id+`"}`)
		}
		r.handle([]byte(`{"type":"event","msg":{"type":"system","subtype":"background_tasks_changed","session_id":"edfe4ade-9f59-4db3-89bc-75fcc8b52c44","tasks":[` + strings.Join(tasks, ",") + `]}}`))
	}
	snapshot("a")
	snapshot()
	snapshot("a")
	if len(sink.background) != 3 || len(sink.background[1]) != 0 || sink.background[2][0].StartedAt.Before(sink.background[0][0].StartedAt) {
		t.Fatalf("background = %+v", sink.background)
	}
	if r.backgroundSeen["a"].IsZero() || len(r.backgroundSeen) != 1 {
		t.Errorf("seen = %v", r.backgroundSeen)
	}
}
