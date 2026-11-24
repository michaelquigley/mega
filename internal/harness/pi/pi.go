// Package pi is the adapter for pi (@earendil-works/pi-coding-agent), driven
// as `pi --mode rpc` over JSONL on stdio, one process per runtime. what pi's
// protocol looks like stays in this package.
//
// pi has no permission model of its own: it runs tools without asking unless
// an extension asks, and under mega that stays exactly so. every dialog an
// extension raises is a question, since nothing marks one as an approval.
// mega never passes --approve, never writes pi's trust, and never answers a
// dialog itself.
package pi

import (
	"context"
	"crypto/rand"
	"fmt"
	"time"

	"github.com/michaelquigley/mega/internal/harness"
	"github.com/michaelquigley/mega/internal/model"
)

const (
	// startTimeout bounds a launch's first answer from pi.
	startTimeout = 2 * time.Minute
	// stopTimeout bounds how long a short-lived pi gets to exit.
	stopTimeout   = 10 * time.Second
	discoverLimit = 50
)

// Config locates the pi executable. Where says where Binary is set, for a
// reason that tells the operator what to do; empty leaves the file unnamed.
type Config struct {
	Binary string
	Where  string
}

// Harness runs pi through a private RPC process per runtime.
type Harness struct {
	cfg Config
}

// New returns the pi harness.
func New(cfg Config) *Harness {
	if cfg.Binary == "" {
		cfg.Binary = "pi"
	}
	return &Harness{cfg: cfg}
}

// Name is model.HarnessPi.
func (h *Harness) Name() string { return model.HarnessPi }

// Check reports whether pi and the interpreter its #!/usr/bin/env line names
// (node) can be found in find.
func (h *Harness) Check(find []string) error {
	where := ""
	if h.cfg.Where != "" {
		where = " " + h.cfg.Where + ","
	}
	return harness.FindExecutable(model.HarnessPi, h.cfg.Binary, find, fmt.Sprintf("install pi, or set pi.binary%s to where it is", where))
}

// Locate names the pi mega would run, for the start-up log.
func (h *Harness) Locate(find []string) string {
	return fmt.Sprintf("pi '%s'", h.Path(find))
}

// Path is the pi mega would run, or the configured one when it cannot be
// found.
func (h *Harness) Path(find []string) string {
	if path, err := harness.LookPath(h.cfg.Binary, find); err == nil {
		return path
	}
	return h.cfg.Binary
}

// Version is what `pi --version` prints: the bare version, such as 1.5.2,
// through the same interpreter mega runs pi with. pi names nothing else, so
// any version-shaped answer is taken.
func (h *Harness) Version(ctx context.Context, find []string) (string, error) {
	argv, err := harness.Program(h.cfg.Binary, find)
	if err != nil {
		return "", err
	}
	return harness.ReadVersion(ctx, append(argv, "--version"), find, "")
}

// Configuration is where the operator's pi configuration is, in env
// (PI_CODING_AGENT_DIR, else ~/.pi/agent), and which of it a configuration
// snapshot copies: settings, custom models and providers (models.json,
// which can hold an API key: configuration, kept as private as the file),
// MCP servers, keybindings, the instruction and system prompt files, and the
// extensions, skills, prompts and themes the operator keeps. never the
// credentials (auth.json, mcp-auth.json), the trust decisions (trust.json),
// the catalogs, caches and logs, the installed packages and tools, or the
// sessions (capability report, 2026-11-16).
func (h *Harness) Configuration(env []string) []harness.ConfigRoot {
	return []harness.ConfigRoot{{
		Harness: model.HarnessPi,
		Label:   "agent",
		Dir:     agentDir(env),
		Include: []string{
			"settings.json", "models.json", "mcp.json", "keybindings.json",
			"AGENTS.md", "AGENTS.override.md", "CLAUDE.md", "SYSTEM.md", "APPEND_SYSTEM.md",
			"extensions/", "skills/", "prompts/", "themes/",
		},
	}}
}

// Launch starts pi on a new session with an id mega mints, or on exactly the
// given one, by its file, with the conversation's model and thinking level.
// pi starts with no --approve: a checkout the operator has not trusted in
// pi's own terminal runs without its project extensions, which the start
// says. the runtime's start is reported before Launch returns.
func (h *Harness) Launch(spec harness.Spec, sink harness.Sink) (harness.Runtime, error) {
	place := harness.Place{Cwd: spec.Cwd, Env: spec.Env, Find: spec.Find}
	id := spec.NativeID
	args := []string{"--mode", "rpc"}
	if id == "" {
		id = newUUID()
		args = append(args, "--session-id", id)
	} else {
		path, err := sessionFile(spec.Env, spec.Cwd, id)
		if err != nil {
			return nil, err
		}
		if err := checkSessionHeader(path, id); err != nil {
			return nil, err
		}
		args = append(args, "--session", path)
	}
	if spec.Model != "" {
		args = append(args, "--model", spec.Model)
	}
	if spec.Effort != "" {
		args = append(args, "--thinking", spec.Effort)
	}
	r := newRuntime(sink)
	c, err := start(h.cfg.Binary, place, args, r)
	if err != nil {
		return nil, err
	}
	r.conn = c
	ctx, cancel := context.WithTimeout(context.Background(), startTimeout)
	defer cancel()
	st := &state{}
	if err := c.call(ctx, map[string]any{"type": "get_state"}, st); err != nil {
		close(r.discard)
		stop(c)
		return nil, err
	}
	if st.SessionID != id {
		close(r.discard)
		stop(c)
		return nil, fmt.Errorf("pi opened session '%s', not '%s'", st.SessionID, id)
	}
	r.mu.Lock()
	r.sessionID = id
	r.model, r.effort = spec.Model, spec.Effort
	r.turnActive = st.IsStreaming
	r.mu.Unlock()
	sink.Event(&model.Event{
		Kind:   model.EventRuntime,
		Status: model.StatusStarted,
		Origin: model.OriginManaged,
		Facts: &model.RuntimeFacts{
			Session: id,
			Model:   st.Model.name(),
			Effort:  st.effort(),
			Cwd:     spec.Cwd,
		},
		Text: trustNotice(spec.Env, spec.Cwd),
	})
	go r.work()
	go r.watch()
	return r, nil
}

// History reads the session's conversation from its file: the chain from the
// last entry back to the root, as pi reopens it.
func (h *Harness) History(_ context.Context, place harness.Place, nativeID string) (harness.History, error) {
	path, err := sessionFile(place.Env, place.Cwd, nativeID)
	if err != nil {
		return harness.History{}, err
	}
	history, err := historyOf(path, nativeID)
	if err != nil {
		return harness.History{}, fmt.Errorf("reading history of '%s': %w", nativeID, err)
	}
	return history, nil
}

// historyOf checks the file’s identity before mapping the same read.
func historyOf(path, want string) (harness.History, error) {
	// one read: the header checked and the entries mapped are the same
	// file's, never a file replaced between two reads.
	header, entries, err := readSession(path)
	if err != nil {
		return harness.History{}, err
	}
	if want != "" {
		if err := harness.SameSession(fmt.Sprintf("the session file '%s'", path), want, header.ID); err != nil {
			return harness.History{}, err
		}
	}
	return harness.History{Events: eventsOf(entries)}, nil
}

// Discover lists the sessions pi stored for the checkout. pi's protocol lists
// none, so it is a scan of the checkout's session folder.
func (h *Harness) Discover(_ context.Context, place harness.Place) ([]harness.Session, error) {
	return discover(place.Env, place.Cwd, discoverLimit)
}

// Models lists the models pi offers in the place, those whose provider has
// credentials, each with the thinking levels it takes, from a short-lived pi
// with no session and no MCP servers. pi writes none of its settings for
// set_model over RPC (pi 1.5.2), so asking each model's levels leaves them as
// they were.
func (h *Harness) Models(ctx context.Context, place harness.Place) (harness.Catalog, error) {
	catalog := harness.Catalog{}
	err := h.inspect(ctx, place, func(c *conn) error {
		current := &state{}
		if err := c.call(ctx, map[string]any{"type": "get_state"}, current); err != nil {
			return err
		}
		listed := &struct{ Models []*modelInfo }{}
		if err := c.call(ctx, map[string]any{"type": "get_available_models"}, listed); err != nil {
			return err
		}
		for _, m := range listed.Models {
			option := harness.ModelOption{ID: m.name(), DisplayName: m.Name, Description: m.Provider, IsDefault: m.name() == current.Model.name()}
			if m.Reasoning {
				levels := &struct{ Levels []string }{}
				if err := c.call(ctx, map[string]any{"type": "set_model", "provider": m.Provider, "modelId": m.ID}, nil); err == nil {
					if err := c.call(ctx, map[string]any{"type": "get_available_thinking_levels"}, levels); err == nil {
						option.Efforts = levels.Levels
					}
				}
				if option.IsDefault {
					option.DefaultEffort = current.ThinkingLevel
				}
			}
			catalog.Models = append(catalog.Models, option)
		}
		return nil
	})
	if err != nil {
		return harness.Catalog{}, err
	}
	catalog.Version, _ = h.Version(ctx, place.Find)
	return catalog, nil
}

// Settings is the model and thinking level pi itself starts a session with
// in the place, as its own settings name them (defaultProvider,
// defaultModel, defaultThinkingLevel) and it resolves them: a level the
// model does not take is clamped by pi, and a model with no reasoning takes
// none.
func (h *Harness) Settings(ctx context.Context, place harness.Place) (model.TurnSettings, error) {
	var out model.TurnSettings
	err := h.inspect(ctx, place, func(c *conn) error {
		st := &state{}
		if err := c.call(ctx, map[string]any{"type": "get_state"}, st); err != nil {
			return err
		}
		out = model.TurnSettings{Model: st.Model.name(), Effort: st.effort()}
		return nil
	})
	return out, err
}

// inspect runs f against a short-lived pi with no session, so nothing is
// written to a session file, and without MCP servers, which a question
// about models or settings does not need.
func (h *Harness) inspect(ctx context.Context, place harness.Place, f func(*conn) error) error {
	c, err := start(h.cfg.Binary, place, []string{"--mode", "rpc", "--no-session", "--no-mcp"}, quiet{})
	if err != nil {
		return err
	}
	defer stop(c)
	return f(c)
}

// quiet ignores what a short-lived pi sends besides responses.
type quiet struct{}

func (quiet) frame(map[string]any) {}

// stop ends a short-lived pi: its input closed, then its process group
// killed if it has not exited in time.
func stop(c *conn) {
	_ = c.closeInput()
	select {
	case <-c.done:
		return
	case <-time.After(stopTimeout):
	}
	c.signal(9, true)
	<-c.done
}

func newUUID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	b[6] = b[6]&0x0f | 0x40
	b[8] = b[8]&0x3f | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:])
}
