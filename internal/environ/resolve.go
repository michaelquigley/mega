package environ

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"syscall"
	"time"
)

// DefaultTimeout bounds one resolution: the shell's start-up files and the
// activation together.
const DefaultTimeout = 10 * time.Second

// systemPath is the PATH the base starts from, as a fresh login session's
// would before the operator's shell start-up files add to it.
const systemPath = "/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin"

// passed are the variables the base keeps from mega's own environment: who
// the operator is, their shell and locale, and the desktop session a terminal
// opened now would inherit. everything else, such as an activated project's
// GOPATH or a devprj marker, is left behind, so a mega started from an
// activated terminal resolves the same as one started from the launcher.
var passed = []string{
	"HOME", "USER", "LOGNAME", "SHELL", "LANG", "LANGUAGE", "TZ",
	"DISPLAY", "WAYLAND_DISPLAY", "XAUTHORITY", "DBUS_SESSION_BUS_ADDRESS",
	"XDG_RUNTIME_DIR", "XDG_SESSION_TYPE", "XDG_SESSION_DESKTOP", "XDG_CURRENT_DESKTOP",
	"SSH_AUTH_SOCK",
}

// Base is the scrubbed environment a resolution's shell starts from, taken
// from environ (mega's own): the passed variables and the locale's LC_*
// variables, and the system PATH.
func Base(environ []string) []string {
	base := []string{"PATH=" + systemPath}
	for _, kv := range environ {
		name, _, ok := strings.Cut(kv, "=")
		if ok && (slices.Contains(passed, name) || strings.HasPrefix(name, "LC_")) {
			base = append(base, kv)
		}
	}
	return base
}

// volatile names change from one shell to the next without meaning anything,
// so they are left out of what is passed on and of the fingerprint.
var volatile = []string{"_", "SHLVL", "OLDPWD"}

// Resolver resolves environments through the operator's shell.
type Resolver struct {
	// Shell is the shell to run, as an absolute path; its name picks the
	// script's dialect (fish, or POSIX for everything else).
	Shell string
	// Base is the environment the shell starts from.
	Base []string
	// Timeout bounds a resolution; zero is DefaultTimeout.
	Timeout time.Duration
	// SkipProfile runs the shell interactive but not as a login shell, so no
	// profile script is read. it is for tests that resolve many times and do
	// not test the profile; mega itself always resolves through a login shell.
	SkipProfile bool
}

// Resolution is an environment as resolved: every variable, in memory only,
// the names the activation changed against the shell's own environment
// before it, and the fingerprint of the whole.
type Resolution struct {
	Env []string
	// Before is the shell's own environment before the activation ran, in
	// memory only. mega finds the programs it runs itself on Before's PATH, so
	// an activation can change what an agent's commands see but never which
	// program mega executes.
	Before      []string
	Changed     []string
	Fingerprint string
	At          time.Time
}

// Failure is a resolution that did not produce an environment: the shell
// could not start, the activation failed or exited, or it ran out of time.
// Output is the tail of what the shell and the activation printed, for the
// operator; it is never recorded.
type Failure struct {
	Reason   string
	Output   string
	TimedOut bool
}

func (f *Failure) Error() string {
	if f.Output == "" {
		return f.Reason
	}
	return f.Reason + ": " + f.Output
}

// ErrFailed marks every resolution failure.
var ErrFailed = errors.New("the environment could not be resolved")

func (f *Failure) Is(target error) bool { return target == ErrFailed }

// Resolve runs the operator's shell interactively in dir from the base,
// applies activate (none is the shell's own environment), and captures the
// environment before and after the activation on their own descriptors, never
// from the shell's output, which start-up files and activations write to.
func (r *Resolver) Resolve(ctx context.Context, dir, activate string) (*Resolution, error) {
	timeout := r.Timeout
	if timeout <= 0 {
		timeout = DefaultTimeout
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	beforeR, beforeW, err := os.Pipe()
	if err != nil {
		return nil, &Failure{Reason: fmt.Sprintf("preparing to resolve: %v", err)}
	}
	afterR, afterW, err := os.Pipe()
	if err != nil {
		beforeR.Close()
		beforeW.Close()
		return nil, &Failure{Reason: fmt.Sprintf("preparing to resolve: %v", err)}
	}
	var output bytes.Buffer
	// a login and interactive shell, so the profile scripts (/etc/profile and
	// /etc/profile.d, the operator's ~/.profile or its shell's equivalent)
	// run as well as the interactive start-up files, as for a terminal the
	// operator's session began in.
	flags := []string{"-l", "-i"}
	if r.SkipProfile {
		flags = []string{"-i"}
	}
	cmd := exec.Command(r.Shell, append(flags, "-c", script(r.Shell, activate))...)
	cmd.Dir = dir
	cmd.Env = r.Base
	cmd.Stdin = nil
	cmd.Stdout = &output
	cmd.Stderr = &output
	cmd.ExtraFiles = []*os.File{beforeW, afterW} // fds 3 and 4
	// its own session, with no controlling terminal: an interactive shell in a
	// background process group of a terminal tries to take that terminal and
	// is stopped until it is killed, as when mega runs from a terminal. a
	// session leader is also its own process group, so the timeout's kill of
	// the group still reaches everything the activation started.
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := cmd.Start(); err != nil {
		beforeR.Close()
		beforeW.Close()
		afterR.Close()
		afterW.Close()
		return nil, &Failure{Reason: fmt.Sprintf("the shell '%s' could not start in '%s': %v", r.Shell, dir, err)}
	}
	beforeW.Close()
	afterW.Close()
	before, after := drain(beforeR), drain(afterR)

	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	var waitErr error
	select {
	case waitErr = <-done:
	case <-ctx.Done():
		// the shell and anything the activation started go together.
		_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		<-done
		<-before
		<-after
		return nil, &Failure{Reason: fmt.Sprintf("the activation in '%s' did not finish within %s", dir, timeout), Output: tail(output.String()), TimedOut: true}
	}
	beforeEnv, afterEnv := <-before, <-after
	if waitErr != nil {
		return nil, &Failure{Reason: fmt.Sprintf("the activation in '%s' failed (%v)", dir, waitErr), Output: tail(output.String())}
	}
	if len(afterEnv) == 0 {
		return nil, &Failure{Reason: fmt.Sprintf("the activation in '%s' ended the shell before its environment could be read", dir), Output: tail(output.String())}
	}
	env := clean(split(afterEnv))
	own := clean(split(beforeEnv))
	return &Resolution{
		Env:         env,
		Before:      own,
		Changed:     changed(own, env),
		Fingerprint: EnvFingerprint(env),
		At:          time.Now().UTC(),
	}, nil
}

// script is what the shell runs: the environment before, the activation, and
// the environment after, on fds 3 and 4. a failing activation (its last
// command's status, as a terminal would see it) ends the shell with that
// status, and nothing is written to fd 4.
func script(shell, activate string) string {
	env := "env"
	if _, err := os.Stat("/usr/bin/env"); err == nil {
		env = "/usr/bin/env"
	}
	if filepath.Base(shell) == "fish" {
		return fmt.Sprintf("%s -0 >&3\n%s\nset -l __mega_status $status\nif test $__mega_status -ne 0; exit $__mega_status; end\n%s -0 >&4\n", env, activate, env)
	}
	return fmt.Sprintf("%s -0 >&3\n%s\n__mega_status=$?\nif [ $__mega_status -ne 0 ]; then exit $__mega_status; fi\n%s -0 >&4\n", env, activate, env)
}

// drain reads a pipe to its end on its own goroutine, so a large environment
// never blocks the shell.
func drain(r *os.File) <-chan []byte {
	out := make(chan []byte, 1)
	go func() {
		defer r.Close()
		data, _ := io.ReadAll(r)
		out <- data
	}()
	return out
}

func split(data []byte) []string {
	var env []string
	for _, kv := range bytes.Split(data, []byte{0}) {
		if len(kv) > 0 && bytes.IndexByte(kv, '=') > 0 {
			env = append(env, string(kv))
		}
	}
	return env
}

// clean drops the volatile variables and sorts by name.
func clean(env []string) []string {
	out := env[:0:0]
	for _, kv := range env {
		name, _, _ := strings.Cut(kv, "=")
		if !slices.Contains(volatile, name) {
			out = append(out, kv)
		}
	}
	slices.Sort(out)
	return out
}

// changed names the variables that differ between before and after: added,
// removed, or given another value. only names; never values.
func changed(before, after []string) []string {
	values := func(env []string) map[string]string {
		m := make(map[string]string, len(env))
		for _, kv := range env {
			name, value, _ := strings.Cut(kv, "=")
			m[name] = value
		}
		return m
	}
	b, a := values(before), values(after)
	var names []string
	for name, value := range a {
		if old, ok := b[name]; !ok || old != value {
			names = append(names, name)
		}
	}
	for name := range b {
		if _, ok := a[name]; !ok {
			names = append(names, name)
		}
	}
	slices.Sort(names)
	return names
}

// EnvFingerprint names a whole environment by its short hash, so two can be
// compared without either being recorded.
func EnvFingerprint(env []string) string {
	return Fingerprint([]byte(strings.Join(env, "\x00")))
}

// noise is what an interactive shell with no terminal says on every start.
var noise = []string{"cannot set terminal process group", "no job control in this shell"}

// tail keeps the last lines the shell printed, without its start-up noise,
// for the operator's message.
func tail(out string) string {
	var lines []string
	for line := range strings.SplitSeq(strings.TrimSpace(out), "\n") {
		if strings.TrimSpace(line) == "" || slices.ContainsFunc(noise, func(n string) bool { return strings.Contains(line, n) }) {
			continue
		}
		lines = append(lines, strings.TrimSpace(line))
	}
	if len(lines) > 6 {
		lines = lines[len(lines)-6:]
	}
	out = strings.Join(lines, "; ")
	if len(out) > 600 {
		out = "…" + out[len(out)-600:]
	}
	return out
}

// Value returns a variable's value from env, and whether it is set.
func Value(env []string, name string) (string, bool) {
	for _, kv := range env {
		if n, v, ok := strings.Cut(kv, "="); ok && n == name {
			return v, true
		}
	}
	return "", false
}
