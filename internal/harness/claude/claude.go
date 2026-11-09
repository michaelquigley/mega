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
	"strings"
	"syscall"
	"time"

	"github.com/michaelquigley/df/dd"

	"git.hq.quigley.com/research/roundhouse/internal/harness"
	"git.hq.quigley.com/research/roundhouse/internal/model"
)

const shimScript = "shim.mjs"

// Config locates node, the shim directory, and the claude code executable.
type Config struct {
	Node       string
	Shim       string
	Executable string
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

func (h *Harness) command(ctx context.Context, args ...string) (*exec.Cmd, error) {
	script, err := h.script()
	if err != nil {
		return nil, err
	}
	cmd := exec.CommandContext(ctx, h.cfg.Node, append([]string{script}, args...)...)
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
	cmd, err := h.command(context.Background(), "run", "--cwd", spec.Cwd, "--exec", h.cfg.Executable, sessionFlag, nativeID)
	if err != nil {
		// the shim was never started, so no session can exist.
		return nil, fmt.Errorf("%w: %w", harness.ErrNotStarted, err)
	}
	r, err := start(cmd, nativeID, sink)
	if err != nil {
		return nil, err
	}
	return r, nil
}

// History reads the session's stored messages through the shim.
func (h *Harness) History(ctx context.Context, cwd, nativeID string) ([]*model.Event, error) {
	cmd, err := h.command(ctx, "messages", "--session", nativeID, "--cwd", cwd)
	if err != nil {
		return nil, err
	}
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("reading history of '%s': %w: %s", nativeID, err, strings.TrimSpace(stderr.String()))
	}
	var evs []*model.Event
	reader := bufio.NewReader(bytes.NewReader(out))
	for {
		line, err := reader.ReadBytes('\n')
		if len(bytes.TrimSpace(line)) > 0 {
			raw, bindErr := dd.DecodeStrictJSON(line)
			if bindErr != nil {
				return nil, fmt.Errorf("reading history of '%s': %w", nativeID, bindErr)
			}
			mapped, mapErr := mapMessage(raw)
			if mapErr != nil {
				return nil, mapErr
			}
			for _, ev := range mapped {
				ev.Origin = model.OriginNative
			}
			evs = append(evs, mapped...)
		}
		if errors.Is(err, io.EOF) {
			return evs, nil
		}
		if err != nil {
			return nil, err
		}
	}
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
func (h *Harness) Discover(ctx context.Context, cwd string) ([]harness.Session, error) {
	cmd, err := h.command(ctx, "list", "--cwd", cwd)
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
			Preview:   clip(preview),
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
