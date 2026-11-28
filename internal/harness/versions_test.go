package harness

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/michaelquigley/mega/internal/model"
)

func TestParseAndCompareVersions(t *testing.T) {
	for out, want := range map[string]string{
		"2.1.325 (Claude Code)\n": "2.1.325",
		"codex-cli 0.169.0":       "0.169.0",
		"fake":                    "",
		"":                        "",
	} {
		if got := ParseVersion(out); got != want {
			t.Errorf("ParseVersion(%q) = %q, want %q", out, got, want)
		}
	}
	for _, c := range []struct {
		a, b string
		want int
	}{
		{"2.1.318", "2.1.325", -1},
		{"2.1.325", "2.1.325", 0},
		{"2.1.330", "2.1.325", 1},
		{"2.10.0", "2.9.9", 1},
		{"0.169.0", "0.163.0", 1},
		{"fake", "2.1.318", -1},
	} {
		if got := CompareVersions(c.a, c.b); got != c.want {
			t.Errorf("CompareVersions(%q, %q) = %d, want %d", c.a, c.b, got, c.want)
		}
	}
}

// a version is tested inside its harness's range, newer or older outside
// it, and unknown when it cannot be read. a newer build, the normal state,
// has no sentence; an older one and an unreadable one have one, an older
// claude code's citing the agent sdk; nothing is refused.
func TestStandAgainstTheTestedRange(t *testing.T) {
	claude, codex := Tested[model.HarnessClaude], Tested[model.HarnessCodex]
	for _, c := range []struct {
		harness, version, standing string
		says                       []string
	}{
		{model.HarnessClaude, claude.Low, StandingTested, nil},
		{model.HarnessClaude, claude.High, StandingTested, nil},
		{model.HarnessClaude, "2.1.999", StandingNewer, nil},
		{model.HarnessClaude, "2.1.300", StandingOlder, []string{"Claude Code '2.1.300' is older", "expects Claude Code '" + claude.Low + "' or later", "may fail"}},
		{model.HarnessCodex, "0.999.0", StandingNewer, nil},
		{model.HarnessCodex, "0.150.0", StandingOlder, []string{"Codex '0.150.0' is older", codex.String()}},
		{model.HarnessClaude, "", StandingUnknown, []string{"could not be read", claude.String()}},
		{model.HarnessCodex, "fake", StandingUnknown, []string{"could not be read"}},
	} {
		standing, note := Stand(c.harness, c.version)
		if standing != c.standing {
			t.Errorf("%s %q: standing %q, want %q", c.harness, c.version, standing, c.standing)
		}
		for _, s := range c.says {
			if !strings.Contains(note, s) {
				t.Errorf("%s %q: note %q does not say %q", c.harness, c.version, note, s)
			}
		}
		if (c.standing == StandingTested || c.standing == StandingNewer) && note != "" {
			t.Errorf("%s %q: a %s version has a note: %q", c.harness, c.version, c.standing, note)
		}
	}
}

// fakeExecutable writes a shell script standing in for a harness's --version.
func fakeExecutable(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "harness")
	if err := os.WriteFile(path, []byte("#!/bin/sh\n"+body+"\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}

// ReadVersion reads what --version printed: a version inside or outside a
// range alike, an exit without one, or with another program's, as an error,
// and a hang as an error once VersionTimeout has passed, stopping the
// process and the children it started.
func TestReadVersion(t *testing.T) {
	saved := VersionTimeout
	VersionTimeout = 300 * time.Millisecond
	t.Cleanup(func() { VersionTimeout = saved })
	env := os.Environ()
	for body, want := range map[string]string{
		`echo "2.1.320 (Claude Code)"`: "2.1.320",
		`echo "2.1.999 (Claude Code)"`: "2.1.999",
		`echo "2.1.100 (Claude Code)"`: "2.1.100",
	} {
		if got, err := ReadVersion(context.Background(), []string{fakeExecutable(t, body), "--version"}, env, "Claude Code"); err != nil || got != want {
			t.Errorf("%s: %q, %v", body, got, err)
		}
	}
	if got, err := ReadVersion(context.Background(), []string{fakeExecutable(t, `echo "codex-cli 0.169.0"`), "--version"}, env, "codex-cli"); err != nil || got != "0.169.0" {
		t.Errorf("codex: %q, %v", got, err)
	}
	for body, says := range map[string]string{
		`echo "Claude Code"`: "printed no version of Claude Code",
		// another program's version, as uutils' false prints for --version.
		`echo "false (uutils coreutils) 0.10.0"; exit 1`: "failed",
		`echo "codex-cli 0.169.0"`:                       "printed no version of Claude Code",
		`echo oops >&2; exit 3`:                          "failed",
	} {
		if got, err := ReadVersion(context.Background(), []string{fakeExecutable(t, body), "--version"}, env, "Claude Code"); err == nil || got != "" || !strings.Contains(err.Error(), says) {
			t.Errorf("%s: %q, %v", body, got, err)
		}
	}
	marker := filepath.Join(t.TempDir(), "child")
	hang := fakeExecutable(t, "sleep 30 & echo $! > "+marker+"; sleep 30")
	start := time.Now()
	got, err := ReadVersion(context.Background(), []string{hang, "--version"}, env, "Claude Code")
	if err == nil || got != "" || !strings.Contains(err.Error(), "did not answer") {
		t.Fatalf("a hang: %q, %v", got, err)
	}
	if took := time.Since(start); took > 3*time.Second {
		t.Errorf("a hang held the read for %s", took)
	}
	// the child was sent SIGKILL with its group; once the kernel has delivered
	// it, which can take a moment, it is gone, or dying ("X (dead)"), or a
	// zombie until reaped.
	pid, _ := os.ReadFile(marker)
	if child := strings.TrimSpace(string(pid)); child != "" {
		dead := func() bool {
			status, err := os.ReadFile("/proc/" + child + "/status")
			return err != nil || strings.Contains(string(status), "zombie") || strings.Contains(string(status), "(dead)")
		}
		gone := dead()
		for deadline := time.Now().Add(2 * time.Second); !gone && time.Now().Before(deadline); gone = dead() {
			time.Sleep(20 * time.Millisecond)
		}
		if !gone {
			t.Errorf("the hang's child is still running")
		}
	}
}
