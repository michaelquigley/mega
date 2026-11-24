package claude

import (
	"bufio"
	"bytes"
	"context"
	"crypto/rand"
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

	"github.com/michaelquigley/df/dd"

	"github.com/michaelquigley/mega/internal/harness"
	"github.com/michaelquigley/mega/internal/model"
)

const shimScript = "shim.mjs"

// Config locates node, the shim directory, and the claude code executable.
// ManagedSettings overrides where managed settings are read; empty is
// DefaultManagedSettings. Where says where these are set, such as "in
// '/home/me/.config/mega/mega.yaml', the config file mega read", for a
// reason that tells the operator what to do; empty leaves the file unnamed.
type Config struct {
	Node            string
	Shim            string
	Executable      string
	ManagedSettings string
	Where           string
	// FileRead is whether a config file was read, so that a configured
	// executable came from it; with none, Where names the file mega looked
	// for, and nothing named the executable there.
	FileRead bool
}

// Harness runs claude code through the stdio shim.
type Harness struct {
	cfg Config
}

// New returns the claude harness.
func New(cfg Config) *Harness {
	return &Harness{cfg: cfg}
}

// Name is model.HarnessClaude.
func (h *Harness) Name() string {
	return model.HarnessClaude
}

// Check reports whether node and the shim can be found: node on find's PATH
// (or at its configured path), the claude executable the same way, the shim
// script and its installed sdk.
func (h *Harness) Check(find []string) error {
	if err := harness.FindExecutable(model.HarnessClaude, h.cfg.Node, find, h.advice("install Node.js", "claude.node")); err != nil {
		return err
	}
	if _, err := h.executable(find); err != nil {
		return err
	}
	if _, err := h.script(); err != nil {
		return &harness.MissingError{Harness: model.HarnessClaude, Executable: "the claude shim", Reason: err.Error()}
	}
	return nil
}

func (h *Harness) script() (string, error) {
	if h.cfg.Shim == "" {
		return "", errors.New("the claude shim directory is not configured (claude.shim)")
	}
	path := filepath.Join(h.cfg.Shim, shimScript)
	if _, err := os.Stat(path); err != nil {
		return "", fmt.Errorf("claude shim: %w", err)
	}
	if _, err := os.Stat(filepath.Join(h.cfg.Shim, "node_modules", "@anthropic-ai", "claude-agent-sdk")); err != nil {
		return "", fmt.Errorf("claude shim dependencies are not installed in '%s' (run make): %w", h.cfg.Shim, err)
	}
	return path, nil
}

// SDKVersion is the version of the Agent SDK installed beside the shim, the
// one its lockfile pins, as that package's own package.json gives it.
func (h *Harness) SDKVersion() (string, error) {
	if h.cfg.Shim == "" {
		return "", errors.New("the claude shim directory is not configured (claude.shim)")
	}
	pkg := &struct{ Version string }{}
	if err := dd.BindJSONFile(pkg, filepath.Join(h.cfg.Shim, "node_modules", "@anthropic-ai", "claude-agent-sdk", "package.json")); err != nil {
		return "", fmt.Errorf("reading the claude agent sdk's version in '%s': %w", h.cfg.Shim, err)
	}
	if pkg.Version == "" {
		return "", fmt.Errorf("the claude agent sdk in '%s' names no version", h.cfg.Shim)
	}
	return pkg.Version, nil
}

// Locate says where claude code, node and the shim were found in find, for
// a harness that can run there.
func (h *Harness) Locate(find []string) string {
	node := h.cfg.Node
	if program, err := harness.Program(h.cfg.Node, find); err == nil {
		node = program[len(program)-1]
	}
	executable, fallback, _ := h.locate(find)
	if fallback {
		return fmt.Sprintf("claude code '%s', the newest build installed, with no '%s' on the PATH, node '%s', shim '%s'", executable, h.cfg.Executable, node, h.cfg.Shim)
	}
	return fmt.Sprintf("claude code '%s', node '%s', shim '%s'", executable, node, h.cfg.Shim)
}

// Path is the claude code executable mega would run, or the configured one
// when it cannot be found.
func (h *Harness) Path(find []string) string {
	if path, _, err := h.locate(find); err == nil {
		return path
	}
	return h.cfg.Executable
}

// Version is the version the claude code mega would run prints for
// --version, with the updater's background check off for that one call: a
// version read never starts an update.
func (h *Harness) Version(ctx context.Context, find []string) (string, error) {
	executable, err := h.executable(find)
	if err != nil {
		return "", err
	}
	return harness.ReadVersion(ctx, []string{executable, "--version"}, append(append([]string{}, find...), "DISABLE_AUTOUPDATER=1"), "Claude Code")
}

// versionsDir is where claude code's native installer keeps its builds,
// under the home find names, else mega's own.
func versionsDir(find []string) string {
	home, ok := lookup(find, "HOME")
	if !ok || home == "" {
		home, _ = os.UserHomeDir()
	}
	return filepath.Join(home, ".local", "share", "claude", "versions")
}

// installed lists the native builds in dir, newest first: executable files
// named by their version, never a script. it reads dir and nothing else.
func installed(dir string) []string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var builds []string
	for _, e := range entries {
		name := e.Name()
		if harness.ParseVersion(name) != name {
			continue
		}
		path := filepath.Join(dir, name)
		if info, err := os.Stat(path); err != nil || !info.Mode().IsRegular() || info.Mode()&0o111 == 0 || script(path) {
			continue
		}
		builds = append(builds, name)
	}
	slices.SortFunc(builds, func(a, b string) int { return harness.CompareVersions(b, a) })
	return builds
}

// advice is what to do about a configured path that cannot run: install the
// thing, or set key, where the config says, to where it is.
func (h *Harness) advice(install, key string) string {
	return fmt.Sprintf("%s, or set %s%s to where it is", install, key, h.where())
}

// where is " in '<file>', …," naming where the config is set, or empty.
func (h *Harness) where() string {
	if h.cfg.Where == "" {
		return ""
	}
	return " " + h.cfg.Where + ","
}

// executable is the claude code executable the shim starts: a configured path
// as it is, checked here so a missing claude code is named before anything
// runs (the sdk itself would fail with a stack trace), or a bare name located
// in find, so the shim never looks it up on the PATH the activation set. it
// must be a native build: the sdk starts a script through an interpreter it
// finds by name on the process's PATH, which the activation sets, so a script
// (a #! line, or a .js, .mjs or .cjs file) is refused rather than handed over.
func (h *Harness) executable(find []string) (string, error) {
	path, _, err := h.locate(find)
	return path, err
}

// locate finds the claude code executable. a configured path is used while
// it is installed; once it is gone, the refusal names the builds installed
// beside it and how to follow the installed one, and says "install" only
// when there is none. a bare name, the default "claude", is looked up on
// find's PATH, and with none there, the newest native build in the
// installer's versions folder is used (fallback): a launcher-started mega
// whose shell lacks ~/.local/bin still finds it.
func (h *Harness) locate(find []string) (path string, fallback bool, err error) {
	path = h.cfg.Executable
	if strings.Contains(path, "/") {
		if err := harness.CheckPath(model.HarnessClaude, path, ""); err != nil {
			if beside := installed(filepath.Dir(path)); len(beside) > 0 {
				follow := "use claude in its place"
				if h.cfg.FileRead {
					follow = "set claude.executable to claude" + h.where() + " or remove it from there"
				}
				return "", false, &harness.MissingError{Harness: model.HarnessClaude, Executable: path, Reason: fmt.Sprintf(
					"'%s' is not installed; the builds installed beside it are %s. to follow the installed Claude Code, %s; or name one of those builds",
					path, strings.Join(beside, ", "), follow)}
			}
			return "", false, harness.CheckPath(model.HarnessClaude, path, h.advice("install Claude Code", "claude.executable"))
		}
	} else {
		found, err := harness.LookPath(path, find)
		if err != nil {
			dir := versionsDir(find)
			builds := installed(dir)
			if len(builds) == 0 {
				return "", false, &harness.MissingError{Harness: model.HarnessClaude, Executable: path, Reason: fmt.Sprintf(
					"'%s' was not found on the project's PATH, and no build is installed in '%s'; %s", path, dir, h.advice("install Claude Code", "claude.executable"))}
			}
			found, fallback = filepath.Join(dir, builds[0]), true
		}
		path = found
	}
	if script(path) {
		return "", false, &harness.MissingError{Harness: model.HarnessClaude, Executable: path, Reason: fmt.Sprintf("'%s' is a script, and the sdk would start it through an interpreter found on the project's activated PATH; set claude.executable to a native build, such as ~/.local/share/claude/versions/<version>", path)}
	}
	return path, fallback, nil
}

// script reports whether path is a script the sdk would run through an
// interpreter: a JavaScript file, or one that starts with #!. a path that
// cannot be read is not called a script; executable has checked it exists.
func script(path string) bool {
	switch filepath.Ext(path) {
	case ".js", ".mjs", ".cjs":
		return true
	}
	f, err := os.Open(path)
	if err != nil {
		return false
	}
	defer f.Close()
	head := make([]byte, 2)
	n, _ := f.Read(head)
	return n == 2 && string(head) == "#!"
}

// command runs the shim with node located in place.Find, the environment
// before the project's activation, and gives it place.Env, the project's whole
// environment, and nothing of mega's own: the activation reaches the claude
// the shim starts and every command claude runs, but never decides which node
// mega runs.
func (h *Harness) command(ctx context.Context, place harness.Place, args ...string) (*exec.Cmd, error) {
	script, err := h.script()
	if err != nil {
		return nil, err
	}
	node, err := harness.Program(h.cfg.Node, place.Find)
	if err != nil {
		var missing *harness.MissingError
		if errors.As(err, &missing) {
			missing.Harness = model.HarnessClaude
		}
		return nil, err
	}
	argv := append(append(node[1:], script), args...)
	cmd := exec.CommandContext(ctx, node[0], argv...)
	// never nil, which exec would fill with mega's own environment.
	cmd.Env = append([]string{}, place.Env...)
	cmd.Dir = h.cfg.Shim
	// its own process group, so a terminal interrupt aimed at the service does
	// not reach the shim and cut short the orderly release.
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	return cmd, nil
}

// Launch starts the shim for a new session or for exactly the given one.
func (h *Harness) Launch(spec harness.Spec, sink harness.Sink) (harness.Runtime, error) {
	nativeID := spec.NativeID
	sessionFlag := "--resume"
	if nativeID == "" {
		nativeID = newUUID()
		sessionFlag = "--session-id"
	}
	executable, err := h.executable(spec.Find)
	if err != nil {
		return nil, err
	}
	args := []string{"run", "--cwd", spec.Cwd, "--exec", executable, sessionFlag, nativeID}
	// the conversation's mode, model and effort, always passed. a mode is
	// always given; were one ever missing, the manual mode is passed, never
	// nothing, so claude's settings cannot pick a looser one. an effort is
	// missing only for a model that takes none.
	mode := spec.PermissionMode
	if mode == "" {
		mode = model.PermissionModeDefault
	}
	args = append(args, "--permission-mode", mode)
	if spec.Model != "" {
		args = append(args, "--model", spec.Model)
	}
	if spec.Effort != "" {
		args = append(args, "--effort", spec.Effort)
	}
	// with no prompt following, claude sends no system/init until a turn
	// starts; the shim reports what claude answers without one.
	if spec.Idle {
		args = append(args, "--idle")
	}
	cmd, err := h.command(context.Background(), harness.Place{Cwd: spec.Cwd, Env: spec.Env, Find: spec.Find}, args...)
	if err != nil {
		return nil, err
	}
	r, err := start(cmd, nativeID, sink, mode, spec.Idle)
	if err != nil {
		return nil, err
	}
	r.mu.Lock()
	r.reqModel, r.reqEffort = spec.Model, spec.Effort
	r.mu.Unlock()
	return r, nil
}

// History reads the exact session’s stored messages through the shim.
func (h *Harness) History(ctx context.Context, place harness.Place, nativeID string) (harness.History, error) {
	cmd, err := h.command(ctx, place, "messages", "--session", nativeID, "--cwd", place.Cwd)
	if err != nil {
		return harness.History{}, err
	}
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return harness.History{}, fmt.Errorf("reading history of '%s': %w: %s", nativeID, err, strings.TrimSpace(stderr.String()))
	}
	var evs []*model.Event
	reader := bufio.NewReader(bytes.NewReader(out))
	for {
		line, err := reader.ReadBytes('\n')
		if len(bytes.TrimSpace(line)) > 0 {
			raw, bindErr := dd.DecodeStrictJSON(line)
			if bindErr != nil {
				return harness.History{}, fmt.Errorf("reading history of '%s': %w", nativeID, bindErr)
			}
			// every message names its session, as every live frame does, and
			// each is checked: messages of another are never imported.
			got, _ := raw["session_id"].(string)
			if err := harness.SameSession("claude's messages", nativeID, got); err != nil {
				return harness.History{}, fmt.Errorf("reading history of '%s': %w", nativeID, err)
			}
			mapped, mapErr := mapSessionMessage(raw, historyUserBlock)
			if mapErr != nil {
				return harness.History{}, mapErr
			}
			for _, ev := range mapped {
				ev.Origin = model.OriginNative
			}
			evs = append(evs, mapped...)
		}
		if errors.Is(err, io.EOF) {
			if len(evs) == 0 {
				// The SDK gives the same empty reply for a missing session.
				sessions, err := h.Discover(ctx, place)
				if err != nil {
					return harness.History{}, err
				}
				if !slices.ContainsFunc(sessions, func(session harness.Session) bool { return session.NativeID == nativeID }) {
					return harness.History{}, fmt.Errorf("session '%s' is unavailable in this checkout", nativeID)
				}
			}
			return harness.History{Events: evs}, nil
		}
		if err != nil {
			return harness.History{}, err
		}
	}
}

// shimModels is what the shim's models command prints.
type shimModels struct {
	Version string `dd:",+nullable"`
	Models  []struct {
		Value                 string   `dd:"value"`
		ResolvedModel         string   `dd:"resolvedModel,+nullable"`
		DisplayName           string   `dd:"displayName,+nullable"`
		Description           string   `dd:"description,+nullable"`
		SupportedEffortLevels []string `dd:"supportedEffortLevels,+nullable"`
	}
}

// Models lists the models claude offers, from an idle session through the
// shim: no prompt is sent, so no turn runs and no session is stored
// (capability report, 2.1.318).
func (h *Harness) Models(ctx context.Context, place harness.Place) (harness.Catalog, error) {
	executable, err := h.executable(place.Find)
	if err != nil {
		return harness.Catalog{}, err
	}
	cmd, err := h.command(ctx, place, "models", "--cwd", place.Cwd, "--exec", executable)
	if err != nil {
		return harness.Catalog{}, err
	}
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return harness.Catalog{}, fmt.Errorf("listing claude models: %w: %s", err, strings.TrimSpace(stderr.String()))
	}
	listed := &shimModels{}
	if err := dd.BindJSON(listed, bytes.TrimSpace(out)); err != nil {
		return harness.Catalog{}, fmt.Errorf("reading claude models: %w", err)
	}
	catalog := harness.Catalog{Version: listed.Version}
	for _, m := range listed.Models {
		// the "default" entry is what claude runs when nothing names a model
		// (capability report, 2.1.318).
		catalog.Models = append(catalog.Models, harness.ModelOption{ID: m.Value, Resolved: m.ResolvedModel, DisplayName: m.DisplayName, Description: m.Description, Efforts: m.SupportedEffortLevels, IsDefault: m.Value == "default"})
	}
	return catalog, nil
}

// sessionInfo is the subset of the sdk's listSessions entries discovery reads.
type sessionInfo struct {
	SessionID    string  `dd:"sessionId"`
	Summary      string  `dd:",+nullable"`
	FirstPrompt  string  `dd:"firstPrompt,+nullable"`
	Cwd          string  `dd:",+nullable"`
	LastModified float64 `dd:"lastModified,+nullable"`
	CreatedAt    float64 `dd:"createdAt,+nullable"`
}

// Discover lists the sessions claude stores for cwd, newest first as the sdk
// returns them.
func (h *Harness) Discover(ctx context.Context, place harness.Place) ([]harness.Session, error) {
	cwd := place.Cwd
	cmd, err := h.command(ctx, place, "list", "--cwd", cwd)
	if err != nil {
		return nil, err
	}
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("listing sessions in '%s': %w: %s", cwd, err, strings.TrimSpace(stderr.String()))
	}
	var sessions []harness.Session
	for _, line := range bytes.Split(out, []byte("\n")) {
		if len(bytes.TrimSpace(line)) == 0 {
			continue
		}
		info := &sessionInfo{}
		if err := dd.BindJSON(info, line); err != nil {
			return nil, fmt.Errorf("reading session list for '%s': %w", cwd, err)
		}
		if info.SessionID == "" || (info.Cwd != "" && info.Cwd != cwd) {
			continue
		}
		preview := info.FirstPrompt
		if preview == "" {
			preview = info.Summary
		}
		sessions = append(sessions, harness.Session{
			NativeID:  info.SessionID,
			Preview:   preview,
			CreatedAt: millis(info.CreatedAt),
			UpdatedAt: millis(info.LastModified),
		})
	}
	return sessions, nil
}

func millis(ms float64) time.Time {
	if ms <= 0 {
		return time.Time{}
	}
	return time.UnixMilli(int64(ms)).UTC()
}

// newUUID mints a version 4 uuid, the form claude requires for a session id.
func newUUID() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		panic(fmt.Errorf("reading random bytes: %w", err))
	}
	b[6] = b[6]&0x0f | 0x40
	b[8] = b[8]&0x3f | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}
