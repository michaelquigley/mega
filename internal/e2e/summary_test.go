//go:build e2e

package e2e

import (
	"context"
	"fmt"
	"os"
	"slices"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/michaelquigley/mega/internal/harness"
	"github.com/michaelquigley/mega/internal/harness/codex"
	"github.com/michaelquigley/mega/internal/service"
)

// the summary `make e2e` ends with: for each harness, the build it ran on,
// how many of its round trips (the map's) passed, failed and were skipped, its
// tested range, and whether the build is admitted, which it is when every one
// of its round trips passed and none was skipped.

// outcome is how one round trip ended.
type outcome string

const (
	passed  outcome = "passed"
	failed  outcome = "failed"
	skipped outcome = "skipped"
)

var record = struct {
	sync.Mutex
	outcomes map[string]outcome
	// installed is each harness's version as mega's own start-up check read
	// it, and reported every version a runtime's start reported.
	installed map[string]map[string]bool
	reported  map[string]map[string]bool
}{
	outcomes:  map[string]outcome{},
	installed: map[string]map[string]bool{},
	reported:  map[string]map[string]bool{},
}

// counted records how the round trip t ends. fixture calls it, first in
// every round trip, so a round trip skipped for want of a fixture counts as
// skipped.
func counted(t *testing.T) {
	t.Helper()
	record.Lock()
	_, seen := record.outcomes[t.Name()]
	if !seen {
		record.outcomes[t.Name()] = failed // until it ends otherwise
	}
	record.Unlock()
	if seen {
		return
	}
	t.Cleanup(func() {
		o := passed
		switch {
		case t.Failed():
			o = failed
		case t.Skipped():
			o = skipped
		}
		record.Lock()
		record.outcomes[t.Name()] = o
		record.Unlock()
	})
}

func note(versions map[string]map[string]bool, harnessName, version string) {
	if version == "" {
		return
	}
	record.Lock()
	defer record.Unlock()
	if versions[harnessName] == nil {
		versions[harnessName] = map[string]bool{}
	}
	versions[harnessName][version] = true
}

// noteBuilds records the versions mega's start-up check read for each
// harness the service holds, and every version a conversation's starts
// reported. a round trip whose starts reported more than one version of a
// harness fails.
func noteBuilds(t *testing.T, svc *service.Service, conversationIDs ...string) {
	t.Helper()
	ws, err := svc.Workspace()
	if err != nil {
		return
	}
	for _, h := range ws.Harnesses {
		note(record.installed, h.Name, h.Version)
	}
	ids := conversationIDs
	if len(ids) == 0 {
		for _, c := range ws.Conversations {
			ids = append(ids, c.ID)
		}
	}
	for _, id := range ids {
		var harnessName string
		for _, c := range ws.Conversations {
			if c.ID == id {
				harnessName = c.Harness
			}
		}
		versions := startedVersions(t, svc, id)
		if len(versions) > 1 {
			t.Errorf("the starts of %s reported more than one version: %v", id, versions)
		}
		for _, v := range versions {
			note(record.reported, harnessName, v)
		}
	}
}

// roundTripsOf is every round trip the map names for a harness.
func roundTripsOf(harnessName string) []string {
	var names []string
	for _, r := range Map {
		if r.Harness != harnessName {
			continue
		}
		for _, name := range r.By {
			if !slices.Contains(names, name) {
				names = append(names, name)
			}
		}
	}
	sort.Strings(names)
	return names
}

func keys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// summarize says where each range stands. a build is admitted when every one
// of its harness's round trips passed, none was skipped or left unrun, and
// one version was seen: the start-up check's, which every start that
// reported a version also reported, and which the run began and ended on.
func summarize(before, after map[string]string) string {
	record.Lock()
	defer record.Unlock()
	var b strings.Builder
	b.WriteString("\nwhere each tested range stands after this make e2e:\n")
	for _, h := range []string{hClaude, hCodex, hPi} {
		counts := map[outcome][]string{}
		var unrun []string
		names := roundTripsOf(h)
		for _, name := range names {
			o, ok := record.outcomes[name]
			if !ok {
				unrun = append(unrun, name)
				continue
			}
			counts[o] = append(counts[o], name)
		}
		installed, reported := keys(record.installed[h]), keys(record.reported[h])
		build := "an unread build"
		if len(installed) > 0 {
			build = strings.Join(installed, " and ")
		}
		var why []string
		switch {
		case before[h] == "" || after[h] == "":
			why = append(why, fmt.Sprintf("the build could not be read before and after the run ('%s', then '%s')", orUnread(before[h]), orUnread(after[h])))
		case before[h] != after[h]:
			why = append(why, fmt.Sprintf("the build changed during the run, from '%s' to '%s'", before[h], after[h]))
		case len(installed) == 1 && before[h] != installed[0]:
			why = append(why, fmt.Sprintf("the run began and ended on '%s'", before[h]))
		}
		switch {
		case len(installed) != 1:
			why = append(why, fmt.Sprintf("the start-up check read %d versions", len(installed)))
		case slices.ContainsFunc(reported, func(v string) bool { return v != installed[0] }):
			why = append(why, fmt.Sprintf("its starts reported '%s'", strings.Join(reported, "', '")))
		}
		if n := len(counts[failed]); n > 0 {
			why = append(why, "failed: "+strings.Join(counts[failed], ", "))
		}
		if n := len(counts[skipped]); n > 0 {
			why = append(why, "skipped: "+strings.Join(counts[skipped], ", "))
		}
		if len(unrun) > 0 {
			why = append(why, "not run: "+strings.Join(unrun, ", "))
		}
		verdict := "admitted"
		if len(why) > 0 {
			verdict = "not admitted; " + strings.Join(why, "; ")
		} else {
			standing, _ := harness.Stand(h, installed[0])
			verdict += fmt.Sprintf(" (%s against the range)", standing)
		}
		fmt.Fprintf(&b, "  %s %s: %d passed, %d failed, %d skipped of %d round trips; tested %s; %s\n",
			h, build, len(counts[passed]), len(counts[failed]), len(counts[skipped]), len(names), harness.Tested[h], verdict)
	}
	return b.String()
}

// builds reads each harness's version as mega's own start-up check does: in
// the operator's shell environment, resolved in the home directory with no
// activation, with the harnesses configured as the round trips configure
// them. a version that cannot be read is left out.
func builds() map[string]string {
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	find := os.Environ()
	if home, err := os.UserHomeDir(); err == nil {
		if res, err := service.DefaultResolver(os.Environ()).Resolve(ctx, home, ""); err == nil {
			find = res.Before
		}
	}
	hs := []harness.Harness{codex.New(codex.Config{Binary: env("MEGA_E2E_CODEX", "codex")}), newPiHarness()}
	if c, err := newClaudeHarness(); err == nil {
		hs = append(hs, c)
	}
	out := map[string]string{}
	for _, h := range hs {
		if v, ok := h.(harness.Versioner); ok {
			if version, err := v.Version(ctx, find); err == nil {
				out[h.Name()] = version
			}
		}
	}
	return out
}

func orUnread(v string) string {
	if v == "" {
		return "unread"
	}
	return v
}

// TestMain reads each harness's build once before the first round trip and
// once after the last, so that a build that changed during the run, as
// claude code updating itself would, is admitted for nothing.
func TestMain(m *testing.M) {
	before := builds()
	code := m.Run()
	after := builds()
	fmt.Print(summarize(before, after))
	os.Exit(code)
}
