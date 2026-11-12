package codex

import (
	"context"
	"crypto/rand"
	"errors"
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
	closeTimeout  = 10 * time.Second
	discoverLimit = 50
)

// Config locates the codex executable.
type Config struct {
	Binary string
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
	c, err := dial(h.cfg.Binary, harness.Place{Cwd: spec.Cwd, Env: spec.Env}, r)
	if err != nil {
		// the app-server never started.
		return nil, fmt.Errorf("%w: %w", harness.ErrNotStarted, err)
	}
	r.conn = c
	ctx, cancel := context.WithTimeout(context.Background(), startTimeout)
	defer cancel()
	started := &threadStarted{}
	if err := c.initialize(ctx); err != nil {
		// no thread is ever created before the handshake completes.
		shutDown(c)
		return nil, fmt.Errorf("%w: %w", harness.ErrNotStarted, err)
	}
	// thread/start and thread/resume take a model but no effort (0.166.0
	// schema); an unchosen model is sent as the config's, so a resumed thread
	// that once had an override returns to the config. the effort goes with
	// every turn/start.
	threadModel := spec.Model
	if threadModel == "" {
		configModel, _, err := readConfig(ctx, c, spec.Cwd)
		if err != nil {
			dl.Warnf("reading what a thread in '%s' inherits: %v", spec.Cwd, err)
		}
		threadModel = configModel
	}
	if spec.NativeID == "" {
		err = c.call(ctx, "thread/start", &threadStartParams{Cwd: spec.Cwd, Model: threadModel}, started)
		var refused *replyError
		if errors.As(err, &refused) {
			// the server answered that it did not start the thread.
			shutDown(c)
			return nil, fmt.Errorf("%w: %w", harness.ErrNotStarted, err)
		}
	} else {
		err = c.call(ctx, "thread/resume", &threadResumeParams{ThreadID: spec.NativeID, Cwd: spec.Cwd, Model: threadModel}, started)
	}
	if err != nil {
		// no answer to thread/start (a timeout, or the app-server exiting) may
		// still have created a thread, so this one is left unclassified.
		shutDown(c)
		return nil, err
	}
	if spec.NativeID != "" && started.Thread.ID != spec.NativeID {
		shutDown(c)
		return nil, fmt.Errorf("resumed thread '%s', not '%s'", started.Thread.ID, spec.NativeID)
	}
	r.mu.Lock()
	r.threadID = started.Thread.ID
	r.cwd, r.model, r.effort = spec.Cwd, spec.Model, spec.Effort
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
	return r, nil
}

// History reads the thread's stored turns from a short-lived app-server. items
// of a turn still in progress are reported open.
func (h *Harness) History(ctx context.Context, place harness.Place, nativeID string) (harness.History, error) {
	read := &threadRead{}
	if err := h.once(ctx, place, "thread/read", &threadReadParams{ThreadID: nativeID, IncludeTurns: true}, read); err != nil {
		return harness.History{}, fmt.Errorf("reading history of '%s': %w", nativeID, err)
	}
	history := harness.History{Open: make(map[string]bool)}
	for _, t := range read.Thread.Turns {
		items, _ := t.Items.([]any)
		for _, entry := range items {
			raw, ok := entry.(map[string]any)
			if !ok {
				continue
			}
			mapped, err := mapItem(raw)
			if err != nil {
				return harness.History{}, err
			}
			for _, ev := range mapped {
				ev.Origin = model.OriginNative
				if t.Status == "inProgress" {
					history.Open[ev.NativeID] = true
				}
			}
			history.Events = append(history.Events, mapped...)
		}
	}
	return history, nil
}

// Discover lists the threads codex stores for the place's checkout.
func (h *Harness) Discover(ctx context.Context, place harness.Place) ([]harness.Session, error) {
	cwd := place.Cwd
	list := &threadList{}
	if err := h.once(ctx, place, "thread/list", &threadListParams{Cwd: cwd, Limit: discoverLimit}, list); err != nil {
		return nil, fmt.Errorf("listing threads in '%s': %w", cwd, err)
	}
	var sessions []harness.Session
	for _, t := range list.Data {
		if t.Cwd != "" && t.Cwd != cwd {
			continue
		}
		sessions = append(sessions, harness.Session{
			NativeID:  t.ID,
			Preview:   t.Preview,
			CreatedAt: seconds(t.CreatedAt),
			UpdatedAt: seconds(t.UpdatedAt),
		})
	}
	return sessions, nil
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

// Models lists the models codex offers, from a short-lived app-server with no
// thread, as discovery does. hidden models are left out. the version comes
// from the app-server's user agent.
// Check reports whether the configured codex binary can be found.
func (h *Harness) Check(env []string) error {
	return harness.FindExecutable(model.HarnessCodex, h.cfg.Binary, env)
}

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

// Inherited says what model and effort a thread with none chosen gets in
// cwd: codex's own effective config for the checkout, project layers
// included, from config/read (capability report, 0.166.0). it only reads.
func (h *Harness) Inherited(ctx context.Context, place harness.Place) (string, string, error) {
	cwd := place.Cwd
	c, err := dial(h.cfg.Binary, place, quiet{})
	if err != nil {
		return "", "", err
	}
	defer shutDown(c)
	if err := c.initialize(ctx); err != nil {
		return "", "", err
	}
	return readConfig(ctx, c, cwd)
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
