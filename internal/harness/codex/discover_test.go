package codex

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/michaelquigley/df/dd"

	"github.com/michaelquigley/mega/internal/harness"
)

// the fake store for thread/list: listed threads in the checkout asked for,
// newest first, three created in each second, plus one in another checkout.
const listedThreads = 250

// fakeThreadList answers thread/list as codex-cli 0.166.0 pages it
// (capability report, 2026-11-26): at most 100 a page whatever is asked;
// under created_at, the default, a cursor that is the last thread's second,
// so a page ending inside a second skips the rest of it; under updated_at a
// position, a simplification of the fake's: codex's own cursor there carries
// milliseconds, and was seen to lose nothing, not promised to. in mode "list-overlap" each page after the first
// begins again with the thread the page before ended on, as a thread updated
// during the read may come round again; in mode "list-endless" every page
// names a next one.
func fakeThreadList(mode string, frame map[string]any) {
	params, _ := frame["params"].(map[string]any)
	cwd, _ := params["cwd"].(string)
	key, _ := params["sortKey"].(string)
	cursor, _ := params["cursor"].(string)
	limit := 25
	if n, ok := params["limit"].(float64); ok {
		limit = int(n)
	}
	limit = min(limit, 100)
	type row struct {
		ID        string `dd:"id"`
		Cwd       string `dd:"cwd"`
		Preview   string `dd:"preview"`
		CreatedAt int64  `dd:"createdAt"`
		UpdatedAt int64  `dd:"updatedAt"`
	}
	var all []row
	for i := range listedThreads {
		all = append(all, row{ID: fmt.Sprintf("t-%03d", i), Cwd: cwd, Preview: fmt.Sprintf("thread %d", i), CreatedAt: 1_800_000_000 - int64(i/3), UpdatedAt: 1_800_000_000 - int64(i)})
	}
	// codex filters by checkout itself; a row from another is dropped by the
	// adapter all the same.
	all = append(all[:100:100], append([]row{{ID: "t-elsewhere", Cwd: "/elsewhere", CreatedAt: all[100].CreatedAt, UpdatedAt: all[100].UpdatedAt}}, all[100:]...)...)
	from := 0
	switch {
	case cursor == "":
	case key == "updated_at":
		from, _ = strconv.Atoi(cursor)
	default:
		second, _ := strconv.ParseInt(cursor, 10, 64)
		from = len(all)
		for i, r := range all {
			if r.CreatedAt < second {
				from = i
				break
			}
		}
	}
	if mode == "list-overlap" && from > 0 {
		from--
	}
	end := min(from+limit, len(all))
	page := all[from:end]
	// no cursor is the last page, as codex's null is.
	next := ""
	switch {
	case mode == "list-endless":
		next = "again"
		page = all[:1]
	case end < len(all) && key == "updated_at":
		next = strconv.Itoa(end)
	case end < len(all):
		next = strconv.FormatInt(page[len(page)-1].CreatedAt, 10)
	}
	type listed struct {
		Data       []row  `dd:"data"`
		NextCursor string `dd:"nextCursor,+omitempty"`
	}
	id, _ := frame["id"].(float64)
	reply, _ := dd.UnbindJSONL(&struct {
		ID     int64   `dd:"id"`
		Result *listed `dd:"result"`
	}{ID: int64(id), Result: &listed{Data: page, NextCursor: next}})
	fmt.Print(string(reply))
}

// discovery follows codex's cursor to the end, sorted by updated_at and not
// by the default whose cursor skips a second's ties: every thread in the
// checkout once, the oldest included, over more than one page of the most
// codex returns. it shows the traversal and the order asked, not that
// codex's own updated_at cursor can never skip.
func TestDiscoverReadsEveryPage(t *testing.T) {
	frames := filepath.Join(t.TempDir(), "frames.jsonl")
	t.Setenv(fakeFrames, frames)
	h := fakeHarness(t, "list")
	place := harness.Place{Cwd: t.TempDir(), Env: os.Environ(), Find: os.Environ()}
	found, err := h.Discover(t.Context(), place)
	if err != nil {
		t.Fatal(err)
	}
	if len(found) != listedThreads {
		t.Fatalf("discovered %d threads, want %d", len(found), listedThreads)
	}
	ids := map[string]bool{}
	for _, s := range found {
		ids[s.NativeID] = true
	}
	if len(ids) != listedThreads || !ids["t-000"] || !ids[fmt.Sprintf("t-%03d", listedThreads-1)] || ids["t-elsewhere"] {
		t.Errorf("discovered %d distinct threads; newest %v, oldest %v, another checkout's %v", len(ids), ids["t-000"], ids[fmt.Sprintf("t-%03d", listedThreads-1)], ids["t-elsewhere"])
	}
	read, err := os.ReadFile(frames)
	if err != nil {
		t.Fatal(err)
	}
	pages := 0
	for _, line := range strings.Split(strings.TrimSpace(string(read)), "\n") {
		frame := map[string]any{}
		if json.Unmarshal([]byte(line), &frame) != nil || frame["method"] != "thread/list" {
			continue
		}
		pages++
		params, _ := frame["params"].(map[string]any)
		if params["sortKey"] != "updated_at" || params["limit"] != float64(100) || params["cwd"] != place.Cwd {
			t.Errorf("page %d asked %v", pages, params)
		}
	}
	if pages != 3 {
		t.Errorf("read %d pages, want 3", pages)
	}
}

// a thread a later page lists again is listed once.
func TestDiscoverListsAThreadOnce(t *testing.T) {
	h := fakeHarness(t, "list-overlap")
	found, err := h.Discover(t.Context(), harness.Place{Cwd: t.TempDir(), Env: os.Environ(), Find: os.Environ()})
	if err != nil {
		t.Fatal(err)
	}
	ids := map[string]bool{}
	for _, s := range found {
		if ids[s.NativeID] {
			t.Errorf("%s listed twice", s.NativeID)
		}
		ids[s.NativeID] = true
	}
	if len(ids) != listedThreads {
		t.Errorf("discovered %d threads, want %d", len(ids), listedThreads)
	}
}

// a list that never ends fails at the caller's deadline; it is never cut
// short and returned as the whole.
func TestDiscoverFailsAtTheDeadlineRatherThanCutTheList(t *testing.T) {
	h := fakeHarness(t, "list-endless")
	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
	defer cancel()
	found, err := h.Discover(ctx, harness.Place{Cwd: t.TempDir(), Env: os.Environ(), Find: os.Environ()})
	if !errors.Is(err, context.DeadlineExceeded) || found != nil {
		t.Fatalf("an endless list returned %d threads, %v", len(found), err)
	}
}
