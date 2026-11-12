package environ

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

// resolver runs bash with a temporary HOME, so no start-up file of the
// operator's is read, and nothing of the test's own environment but PATH.
func resolver(t *testing.T) *Resolver {
	t.Helper()
	bash, err := lookBash()
	if err != nil {
		t.Skip("bash is not installed")
	}
	return &Resolver{Shell: bash, Base: []string{"HOME=" + t.TempDir(), "PATH=" + systemPath}, Timeout: 5 * time.Second}
}

func lookBash() (string, error) {
	for _, p := range []string{"/bin/bash", "/usr/bin/bash"} {
		if _, err := os.Stat(p); err == nil {
			return p, nil
		}
	}
	return "", errors.New("no bash")
}

func TestBaseKeepsTheSessionAndDropsAnActivation(t *testing.T) {
	base := Base([]string{
		"HOME=/home/op", "USER=op", "SHELL=/bin/bash", "LC_ALL=C.UTF-8", "DISPLAY=:0",
		"PATH=/home/op/local/gopaths/other/bin:/usr/bin", "GOPATH=/home/op/local/gopaths/other",
		"_DEVPRJ_NAME=other", "CLAUDE_CONFIG_DIR=/tmp/scratch",
	})
	for _, want := range []string{"HOME=/home/op", "USER=op", "SHELL=/bin/bash", "LC_ALL=C.UTF-8", "DISPLAY=:0", "PATH=" + systemPath} {
		if !slices.Contains(base, want) {
			t.Errorf("base lacks %q: %v", want, base)
		}
	}
	for _, gone := range []string{"GOPATH", "_DEVPRJ_NAME", "CLAUDE_CONFIG_DIR"} {
		if _, ok := Value(base, gone); ok {
			t.Errorf("base keeps %s", gone)
		}
	}
}

func TestAnActivationIsAppliedAndOnlyItsNamesReported(t *testing.T) {
	r := resolver(t)
	dir := t.TempDir()
	one, err := r.Resolve(context.Background(), dir, "export MEGA_MARKER=one\necho 'activated, say so on stdout'")
	if err != nil {
		t.Fatal(err)
	}
	if v, _ := Value(one.Env, "MEGA_MARKER"); v != "one" {
		t.Errorf("the marker is %q", v)
	}
	if !slices.Equal(one.Changed, []string{"MEGA_MARKER"}) {
		t.Errorf("changed names: %v", one.Changed)
	}
	if v, _ := Value(one.Env, "PWD"); v != dir {
		t.Errorf("the shell ran in %q, not the checkout", v)
	}
	two, err := r.Resolve(context.Background(), dir, "export MEGA_MARKER=two")
	if err != nil {
		t.Fatal(err)
	}
	if one.Fingerprint == two.Fingerprint || !strings.HasPrefix(one.Fingerprint, "sha256:") {
		t.Errorf("fingerprints %q and %q", one.Fingerprint, two.Fingerprint)
	}
	again, err := r.Resolve(context.Background(), dir, "export MEGA_MARKER=one")
	if err != nil {
		t.Fatal(err)
	}
	if again.Fingerprint != one.Fingerprint {
		t.Errorf("the same activation fingerprints differently: %q and %q", one.Fingerprint, again.Fingerprint)
	}
	none, err := r.Resolve(context.Background(), dir, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(none.Changed) != 0 {
		t.Errorf("no activation changed %v", none.Changed)
	}
	if _, ok := Value(none.Env, "MEGA_MARKER"); ok {
		t.Error("no activation still has the marker")
	}
}

func TestAFailedActivationIsAFailure(t *testing.T) {
	r := resolver(t)
	for _, tc := range []struct {
		name, activate, says string
	}{
		{"a failing command", "echo 'devprj: no .devprj here' >&2; false", "devprj: no .devprj here"},
		{"an exit", "exit 3", "failed"},
		{"an exit with success", "exit 0", "ended the shell"},
	} {
		_, err := r.Resolve(context.Background(), t.TempDir(), tc.activate)
		var failure *Failure
		if !errors.As(err, &failure) || !errors.Is(err, ErrFailed) || !strings.Contains(err.Error(), tc.says) {
			t.Errorf("%s: %v", tc.name, err)
		}
	}
}

func TestATimedOutActivationIsStopped(t *testing.T) {
	r := resolver(t)
	r.Timeout = 300 * time.Millisecond
	start := time.Now()
	_, err := r.Resolve(context.Background(), t.TempDir(), "sleep 30 & wait")
	var failure *Failure
	if !errors.As(err, &failure) || !failure.TimedOut {
		t.Fatalf("a slow activation: %v", err)
	}
	if took := time.Since(start); took > 3*time.Second {
		t.Errorf("the timeout took %s", took)
	}
}

func TestTheFileIsReadWithItsFingerprint(t *testing.T) {
	checkout := t.TempDir()
	if f := ReadFile(checkout); f.Present || f.Malformed != "" {
		t.Errorf("no file: %+v", f)
	}
	if err := os.MkdirAll(filepath.Join(checkout, ".mega"), 0o755); err != nil {
		t.Fatal(err)
	}
	write := func(body string) File {
		t.Helper()
		if err := os.WriteFile(filepath.Join(checkout, FilePath), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
		return ReadFile(checkout)
	}
	f := write("activate: devprj\n# later keys describe more\ncontainer: none\n")
	if !f.Present || f.Activate != "devprj" || f.Malformed != "" || f.Fingerprint != Fingerprint([]byte(f.Content)) || f.ModifiedAt.IsZero() {
		t.Errorf("a valid file: %+v", f)
	}
	if g := write("activate: devprj\n# later keys describe more\ncontainer: none\n"); g.Fingerprint != f.Fingerprint {
		t.Error("the same content fingerprints differently")
	}
	if g := write("activate: [devprj]\n"); g.Malformed == "" || !strings.Contains(g.Malformed, FilePath) {
		t.Errorf("a list for activate: %+v", g)
	}
	if g := write("activate: devprj\n  bad: indent\n:"); g.Malformed == "" {
		t.Errorf("not yaml: %+v", g)
	}
}

// the shell is a login shell too: a variable set only in a profile script,
// as cargo, Homebrew on Linux or Nix set theirs, is in the environment.
func TestAProfileScriptsVariableSurvives(t *testing.T) {
	r := resolver(t)
	home, _ := Value(r.Base, "HOME")
	if err := os.WriteFile(filepath.Join(home, ".profile"), []byte("export MEGA_PROFILE_ONLY=from-the-profile\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	res, err := r.Resolve(context.Background(), t.TempDir(), "")
	if err != nil {
		t.Fatal(err)
	}
	if v, _ := Value(res.Env, "MEGA_PROFILE_ONLY"); v != "from-the-profile" {
		t.Errorf("a profile script's variable is %q", v)
	}
	if v, _ := Value(res.Before, "MEGA_PROFILE_ONLY"); v != "from-the-profile" {
		t.Errorf("before the activation, the profile's variable is %q", v)
	}
}

// the environment before the activation is kept, so mega can find its own
// programs where the activation cannot have put them.
func TestTheEnvironmentBeforeTheActivationIsKept(t *testing.T) {
	r := resolver(t)
	res, err := r.Resolve(context.Background(), t.TempDir(), "export PATH=/planted/by/the/activation:$PATH")
	if err != nil {
		t.Fatal(err)
	}
	after, _ := Value(res.Env, "PATH")
	before, _ := Value(res.Before, "PATH")
	if !strings.HasPrefix(after, "/planted/by/the/activation:") || strings.Contains(before, "/planted") {
		t.Errorf("PATH after %q, before %q", after, before)
	}
	if !slices.Equal(res.Changed, []string{"PATH"}) {
		t.Errorf("changed: %v", res.Changed)
	}
}

func TestTheDigestIsFullAndTheFingerprintShort(t *testing.T) {
	d := Digest([]byte("activate: devprj\n"))
	if !IsDigest(d) || len(d) != len("sha256:")+64 {
		t.Errorf("digest %q", d)
	}
	f := Fingerprint([]byte("activate: devprj\n"))
	if f != Short(d) || len(f) != len("sha256:")+16 || IsDigest(f) {
		t.Errorf("fingerprint %q of digest %q", f, d)
	}
	for _, not := range []string{"", "sha256:", f, "sha256:" + strings.Repeat("z", 64), "md5:" + strings.Repeat("a", 64)} {
		if IsDigest(not) {
			t.Errorf("%q reads as a digest", not)
		}
	}
}
