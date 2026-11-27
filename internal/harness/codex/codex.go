package codex

import (
	"context"
	"crypto/rand"
	"fmt"
	"strings"
	"syscall"
	"time"

	"github.com/michaelquigley/df/dl"

	"github.com/michaelquigley/mega/internal/harness"
	"github.com/michaelquigley/mega/internal/model"
)

const (
	// startTimeout bounds the handshake and thread start or resume, which
	// includes MCP startup.
	startTimeout = 2 * time.Minute
	// closeTimeout bounds how long a short-lived app-server gets to exit.
	closeTimeout = 10 * time.Second
	// listPage is the most threads codex returns in one thread/list page,
	// whatever is asked (codex-cli 0.166.0, capability report).
	listPage = 100
	// listSort is the order discovery pages in. under created_at, codex's
	// cursor is a whole second, and a page that ends inside a second skips
	// the rest of it; under updated_at it carries milliseconds, and the
	// fixture's list was read whole at every page size tried. observed, not
	// promised (capability report, 2026-11-26).
	listSort = "updated_at"
)

// Config locates the codex executable.
// Where says where Binary is set, such as "in '/home/me/.config/mega/
// mega.yaml', the config file mega read", for a reason that tells the operator
// what to do; empty leaves the file unnamed.
type Config struct {
	Binary string
	Where  string
}

// Harness runs codex through a private app-server per runtime.
type Harness struct {
	cfg Config
}

// New returns the codex harness.
func New(cfg Config) *Harness {
	if cfg.Binary == "" {
		cfg.Binary = "codex"
	}
	return &Harness{cfg: cfg}
}

// Name is model.HarnessCodex.
func (h *Harness) Name() string {
	return model.HarnessCodex
}

// Launch starts an app-server, then starts a thread or resumes exactly the
// given one, with no approval policy or sandbox override: the operator's codex
// configuration decides both. the runtime's start is reported before Launch
// returns.
func (h *Harness) Launch(spec harness.Spec, sink harness.Sink) (harness.Runtime, error) {
	r := newRuntime(sink)
	r.expected = spec.NativeID
	c, err := dial(h.cfg.Binary, harness.Place{Cwd: spec.Cwd, Env: spec.Env, Find: spec.Find}, r)
	if err != nil {
		return nil, err
	}
	r.conn = c
	ctx, cancel := context.WithTimeout(context.Background(), startTimeout)
	defer cancel()
	started := &threadStarted{}
	if err := c.initialize(ctx); err != nil {
		shutDown(c)
		return nil, err
	}
	// thread/start and thread/resume take a model but no effort (0.166.0
	// schema): the conversation's model goes with them, and its effort with
	// every turn/start.
	threadModel := spec.Model
	if spec.NativeID == "" {
		err = c.call(ctx, "thread/start", &threadStartParams{Cwd: spec.Cwd, Model: threadModel}, started)
	} else {
		err = c.call(ctx, "thread/resume", &threadResumeParams{ThreadID: spec.NativeID, Cwd: spec.Cwd, Model: threadModel}, started)
	}
	if err != nil {
		shutDown(c)
		return nil, err
	}
	if spec.NativeID != "" && started.Thread.ID != spec.NativeID {
		shutDown(c)
		return nil, fmt.Errorf("resumed thread '%s', not '%s'", started.Thread.ID, spec.NativeID)
	}
	r.mu.Lock()
	r.threadID = started.Thread.ID
	r.model, r.effort = spec.Model, spec.Effort
	r.liveModel, r.liveEffort = started.Model, started.ReasoningEffort
	r.mu.Unlock()
	sink.Event(&model.Event{
		Kind:   model.EventRuntime,
		Status: model.StatusStarted,
		Origin: model.OriginManaged,
		Facts: &model.RuntimeFacts{
			Session:        started.Thread.ID,
			Model:          started.Model,
			Version:        started.Thread.CliVersion,
			PermissionMode: permissionSummary(started),
			Effort:         started.ReasoningEffort,
			Cwd:            started.Cwd,
		},
		Text: instructionSummary(started.InstructionSources),
	})
	go r.watch()
	// the thread's MCP servers, as they stand now; startup notifications
	// that came before the thread was known asked for nothing.
	r.refreshMCP()
	return r, nil
}

// History reads the exact thread’s stored turns from a short-lived app-server.
func (h *Harness) History(ctx context.Context, place harness.Place, nativeID string) (harness.History, error) {
	read := &threadRead{}
	if err := h.once(ctx, place, "thread/read", &threadReadParams{ThreadID: nativeID, IncludeTurns: true}, read); err != nil {
		return harness.History{}, fmt.Errorf("reading history of '%s': %w", nativeID, err)
	}
	// another thread than the one asked for: nothing of it is imported.
	if err := harness.SameSession("codex's thread/read", nativeID, read.Thread.ID); err != nil {
		return harness.History{}, fmt.Errorf("reading history of '%s': %w", nativeID, err)
	}
	var history harness.History
	for _, t := range read.Thread.Turns {
		items, _ := t.Items.([]any)
		for _, entry := range items {
			raw, ok := entry.(map[string]any)
			if !ok {
				continue
			}
			if status, _ := raw["status"].(string); status == "inProgress" {
				// a command or tool call still running, which can outlive its
				// turn: only its completion produces a result, under the id it
				// will carry then.
				continue
			}
			mapped, err := mapItem(raw)
			if err != nil {
				return harness.History{}, err
			}
			for _, ev := range mapped {
				ev.Origin = model.OriginNative
			}
			history.Events = append(history.Events, mapped...)
		}
	}
	return history, nil
}

// Discover reads codex's listing for the place's checkout page by page to
// the end: every thread codex lists by default, which leaves out archived
// threads and those from non-interactive sources such as codex exec. a
// thread already listed is not listed again. only the caller's deadline
// bounds the read, so a list that never ends fails and is never cut short.
func (h *Harness) Discover(ctx context.Context, place harness.Place) ([]harness.Session, error) {
	cwd := place.Cwd
	c, err := dial(h.cfg.Binary, place, quiet{})
	if err != nil {
		return nil, fmt.Errorf("listing threads in '%s': %w", cwd, err)
	}
	defer shutDown(c)
	if err := c.initialize(ctx); err != nil {
		return nil, fmt.Errorf("listing threads in '%s': %w", cwd, err)
	}
	var sessions []harness.Session
	seen := map[string]bool{}
	params := &threadListParams{Cwd: cwd, Limit: listPage, SortKey: listSort}
	for {
		list := &threadList{}
		if err := c.call(ctx, "thread/list", params, list); err != nil {
			return nil, fmt.Errorf("listing threads in '%s': %w", cwd, err)
		}
		for _, t := range list.Data {
			if (t.Cwd != "" && t.Cwd != cwd) || seen[t.ID] {
				continue
			}
			seen[t.ID] = true
			sessions = append(sessions, harness.Session{
				NativeID:  t.ID,
				Preview:   t.Preview,
				CreatedAt: seconds(t.CreatedAt),
				UpdatedAt: seconds(t.UpdatedAt),
			})
		}
		if list.NextCursor == "" {
			return sessions, nil
		}
		params.Cursor = list.NextCursor
	}
}

// once runs one call against a short-lived app-server with no thread loaded.
func (h *Harness) once(ctx context.Context, place harness.Place, method string, params, out any) error {
	c, err := dial(h.cfg.Binary, place, quiet{})
	if err != nil {
		return err
	}
	defer shutDown(c)
	if err := c.initialize(ctx); err != nil {
		return err
	}
	return c.call(ctx, method, params, out)
}

// Check reports whether the configured codex can run: a path is checked as
// configured, with what to do if it cannot run, and a bare name on find's
// PATH. it must be installed whole, with the helper programs its package
// names beside it (helpers).
func (h *Harness) Check(find []string) error {
	where := ""
	if h.cfg.Where != "" {
		where = " " + h.cfg.Where + ","
	}
	if err := harness.FindExecutable(model.HarnessCodex, h.cfg.Binary, find, fmt.Sprintf("install Codex, or set codex.binary%s to where it is", where)); err != nil {
		return err
	}
	path, err := h.resolve(find)
	if err != nil {
		return err
	}
	return h.helpers(path)
}

// Locate says where codex was found in find, for a harness that can run
// there.
func (h *Harness) Locate(find []string) string {
	return fmt.Sprintf("codex '%s'", h.Path(find))
}

// Path is the codex mega would run, or the configured one when it cannot be
// found.
func (h *Harness) Path(find []string) string {
	if path, err := h.resolve(find); err == nil {
		return path
	}
	return h.cfg.Binary
}

// Models lists the models codex offers, from a short-lived app-server with no
// thread, as discovery does. hidden models are left out. the version comes
// from the app-server's user agent.

func (h *Harness) Models(ctx context.Context, place harness.Place) (harness.Catalog, error) {
	c, err := dial(h.cfg.Binary, place, quiet{})
	if err != nil {
		return harness.Catalog{}, err
	}
	defer shutDown(c)
	if err := c.initialize(ctx); err != nil {
		return harness.Catalog{}, err
	}
	list := &modelList{}
	if err := c.call(ctx, "model/list", &modelListParams{}, list); err != nil {
		return harness.Catalog{}, fmt.Errorf("listing codex models: %w", err)
	}
	catalog := harness.Catalog{Version: userAgentVersion(c.userAgent)}
	for _, m := range list.Data {
		if m.Hidden {
			continue
		}
		option := harness.ModelOption{ID: m.ID, Resolved: m.ID, DisplayName: m.DisplayName, Description: m.Description, DefaultEffort: m.DefaultReasoningEffort, IsDefault: m.IsDefault}
		efforts, _ := m.SupportedReasoningEfforts.([]any)
		for _, e := range efforts {
			switch v := e.(type) {
			case string:
				option.Efforts = append(option.Efforts, v)
			case map[string]any:
				if s, ok := v["reasoningEffort"].(string); ok {
					option.Efforts = append(option.Efforts, s)
				}
			}
		}
		catalog.Models = append(catalog.Models, option)
	}
	return catalog, nil
}

// Settings says what model and effort codex's own effective config gives a
// thread in cwd, project layers included, from config/read (capability
// report, 0.166.0): what a new conversation copies once. codex has no
// permission mode. it only reads.
func (h *Harness) Settings(ctx context.Context, place harness.Place) (model.TurnSettings, error) {
	c, err := dial(h.cfg.Binary, place, quiet{})
	if err != nil {
		return model.TurnSettings{}, err
	}
	defer shutDown(c)
	if err := c.initialize(ctx); err != nil {
		return model.TurnSettings{}, err
	}
	configModel, configEffort, err := readConfig(ctx, c, place.Cwd)
	if err != nil {
		return model.TurnSettings{}, err
	}
	return model.TurnSettings{Model: configModel, Effort: configEffort}, nil
}

// readConfig reads the model and effort codex's effective config gives cwd,
// project layers included, over an initialized connection. a field the
// config does not set is empty.
func readConfig(ctx context.Context, c *conn, cwd string) (string, string, error) {
	read := &configRead{}
	if err := c.call(ctx, "config/read", &configReadParams{Cwd: cwd}, read); err != nil {
		return "", "", fmt.Errorf("reading codex config: %w", err)
	}
	model, _ := read.Config["model"].(string)
	effort, _ := read.Config["model_reasoning_effort"].(string)
	return model, effort, nil
}

// userAgentVersion reads the version from an app-server user agent such as
// "mega/0.166.0 (Ubuntu ...)".
func userAgentVersion(agent string) string {
	head, _, _ := strings.Cut(agent, " ")
	_, version, _ := strings.Cut(head, "/")
	return version
}

// shutDown closes a connection's input and waits for the app-server to exit,
// ending its process group if it does not.
func shutDown(c *conn) {
	if err := c.closeInput(); err != nil {
		dl.Debugf("closing codex app-server input: %v", err)
	}
	select {
	case <-c.done:
	case <-time.After(closeTimeout):
		dl.Warnf("codex app-server %d did not exit; ending its process group", c.cmd.Process.Pid)
		if err := syscall.Kill(-c.cmd.Process.Pid, syscall.SIGTERM); err != nil {
			dl.Warnf("ending codex app-server %d: %v", c.cmd.Process.Pid, err)
		}
		<-c.done
	}
}

// quiet ignores what a short-lived app-server sends besides responses. no
// thread is loaded, so no server request can arrive.
type quiet struct{}

func (quiet) notify(string, map[string]any)       {}
func (quiet) request(any, string, map[string]any) {}

func permissionSummary(started *threadStarted) string {
	var parts []string
	switch policy := started.ApprovalPolicy.(type) {
	case string:
		parts = append(parts, "approval "+policy)
	case nil:
	default:
		parts = append(parts, fmt.Sprintf("approval %v", policy))
	}
	if kind, ok := started.Sandbox["type"].(string); ok {
		parts = append(parts, "sandbox "+kind)
	}
	return strings.Join(parts, ", ")
}

func instructionSummary(sources []string) string {
	if len(sources) == 0 {
		return ""
	}
	return "instructions: " + strings.Join(sources, ", ")
}

func seconds(s int64) time.Time {
	if s <= 0 {
		return time.Time{}
	}
	return time.Unix(s, 0).UTC()
}

// newID mints a version 4 uuid for a message identity codex records as the
// user message's clientId.
func newID() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		panic(fmt.Errorf("reading random bytes: %w", err))
	}
	b[6] = b[6]&0x0f | 0x40
	b[8] = b[8]&0x3f | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}
