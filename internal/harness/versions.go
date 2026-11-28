package harness

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/michaelquigley/mega/internal/model"
)

// Range is a harness's tested range, lowest and highest. the upper end is
// the newest build `make e2e` admitted: every one of that harness's round
// trips, the ones internal/e2e's map names, passed on it and none was
// skipped. the lower end is the oldest build tested, by the tests of its day:
// the version probe for claude code and codex (probes/versions/README.md),
// the pi probes and round trips for pi. so only the upper end is known
// to pass today's round trips, and no build between the ends is promised to.
type Range struct {
	Low  string
	High string
}

func (r Range) String() string { return fmt.Sprintf("%s to %s", r.Low, r.High) }

// Tested is each harness's tested range, the one place mega keeps them. a
// build enters its range when `make e2e` admits it, as the summary that run
// ends with says, and is widened here in the same change, with the summary
// line in the day's journal. claude code is with the shim's pinned agent
// sdk; codex is installed whole, with its helper programs.
var Tested = map[string]Range{
	model.HarnessClaude: {Low: "2.1.318", High: "2.1.331"},
	model.HarnessCodex:  {Low: "0.163.0", High: "0.172.0"},
	model.HarnessPi:     {Low: "1.5.2", High: "1.6.1"},
}

// labels are the harnesses' names in a sentence.
var labels = map[string]string{
	model.HarnessClaude: "Claude Code",
	model.HarnessCodex:  "Codex",
	model.HarnessPi:     "pi",
}

// standings: where a version stands against its harness's tested range.
const (
	StandingTested  = "tested"
	StandingNewer   = "newer"
	StandingOlder   = "older"
	StandingUnknown = "unknown"
)

// VersionTimeout bounds a `--version`: every build seen answers in about 10
// ms, so a build that has not answered in this is reported as unknown and
// mega goes on without it.
var VersionTimeout = 2 * time.Second

var semver = regexp.MustCompile(`(\d+)\.(\d+)\.(\d+)`)

// ParseVersion is the first x.y.z in what a `--version` printed, or empty:
// "2.1.325 (Claude Code)" is 2.1.325, "codex-cli 0.169.0" is 0.169.0.
func ParseVersion(out string) string {
	return semver.FindString(out)
}

func parts(version string) ([3]int, bool) {
	m := semver.FindStringSubmatch(version)
	if m == nil {
		return [3]int{}, false
	}
	var p [3]int
	for i := range p {
		p[i], _ = strconv.Atoi(m[i+1])
	}
	return p, true
}

// CompareVersions orders two x.y.z versions: -1, 0 or 1. a version that is
// not one sorts first.
func CompareVersions(a, b string) int {
	pa, oka := parts(a)
	pb, okb := parts(b)
	switch {
	case !oka && !okb:
		return 0
	case !oka:
		return -1
	case !okb:
		return 1
	}
	for i := range pa {
		switch {
		case pa[i] < pb[i]:
			return -1
		case pa[i] > pb[i]:
			return 1
		}
	}
	return 0
}

// Stand says where a harness's version stands against its tested range, and
// a sentence for the operator when it needs a look: an older build, since
// claude's agent sdk expects the claude code of its own patch or later, and
// a version that could not be read. a newer build has none: claude code
// releases every day or two, so an installed build is above the range almost
// always, and that is the normal state, said in plain words. nothing is
// refused for its version.
func Stand(harnessName, version string) (standing, note string) {
	r, ok := Tested[harnessName]
	label := labels[harnessName]
	if label == "" {
		label = harnessName
	}
	switch {
	case !ok:
		return StandingUnknown, ""
	case ParseVersion(version) == "":
		return StandingUnknown, fmt.Sprintf("the version of %s could not be read, so mega cannot say whether it was tested; mega was tested with '%s'", label, r)
	case CompareVersions(version, r.Low) < 0:
		if harnessName == model.HarnessClaude {
			return StandingOlder, fmt.Sprintf("%s '%s' is older than the builds mega was tested with, '%s'; the Agent SDK mega runs expects Claude Code '%s' or later, so some calls may fail", label, version, r, r.Low)
		}
		return StandingOlder, fmt.Sprintf("%s '%s' is older than the builds mega was tested with, '%s'; some calls mega makes may not be understood", label, version, r)
	case CompareVersions(version, r.High) > 0:
		return StandingNewer, ""
	}
	return StandingTested, ""
}

// Versioner is a harness that can say which version of it is installed, as
// its own `--version` prints it, where find locates its programs.
type Versioner interface {
	Version(ctx context.Context, find []string) (string, error)
}

// ReadVersion runs argv (a program and its --version) with env and returns
// the version it printed, when what it printed names the harness as says
// (claude code prints "2.1.325 (Claude Code)", codex "codex-cli 0.169.0"):
// another program's version, such as a /bin/false that prints its own, is
// not taken for the harness's. it gives up after VersionTimeout, stopping the
// process and its children; a version it cannot read is an error.
func ReadVersion(ctx context.Context, argv []string, env []string, says string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, VersionTimeout)
	defer cancel()
	cmd := exec.Command(argv[0], argv[1:]...)
	cmd.Env = env
	var out bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &out
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := cmd.Start(); err != nil {
		return "", err
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case err := <-done:
		if v := ParseVersion(out.String()); v != "" && strings.Contains(out.String(), says) {
			return v, nil
		}
		if err != nil {
			return "", fmt.Errorf("'%s --version' failed: %w", argv[0], err)
		}
		return "", fmt.Errorf("'%s --version' printed no version of %s", argv[0], says)
	case <-ctx.Done():
		_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		<-done
		return "", errors.New("'" + argv[0] + " --version' did not answer within " + VersionTimeout.String())
	}
}
