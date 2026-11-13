package claude

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/michaelquigley/df/dd"
	"github.com/michaelquigley/df/dl"

	"github.com/michaelquigley/mega/internal/harness"
	"github.com/michaelquigley/mega/internal/model"
)

// DefaultManagedSettings is where claude code reads managed settings on linux.
const DefaultManagedSettings = "/etc/claude-code/managed-settings.json"

type claudeSettings struct {
	Permissions struct {
		DefaultMode string `dd:"defaultMode,+omitempty"`
	} `dd:",+omitempty"`
	Model       string `dd:"model,+omitempty"`
	EffortLevel string `dd:"effortLevel,+omitempty"`
}

// settingsLayer is one settings file claude merges. project marks the
// checkout's own files, from which claude does not honor every mode.
type settingsLayer struct {
	path    string
	project bool
}

// ignoredInProject are the defaultMode values claude 2.1.318 does not take
// from project or local settings, only from user or managed ones (observed;
// capability report, 2026-11-09).
var ignoredInProject = map[string]bool{"auto": true, "bypassPermissions": true}

// settingsMode returns the permissions.defaultMode the operator's claude
// settings name for a session in cwd, as claude 2.1.318 resolves it, or ""
// when none does. it reads, highest precedence first: managed settings; the
// local settings.local.json at the repository root, then the one in cwd; the
// shared .claude/settings.json in cwd (never the repository root's, which
// claude does not read for a session below it); then the user's settings.json
// ($CLAUDE_CONFIG_DIR, else ~/.claude, as env, the project's resolved
// environment, names them; with none, mega's home). auto and bypassPermissions
// named in a project or local file are skipped, as claude ignores them there.
// it only reads. a file that cannot be read is skipped with a warning.
func (h *Harness) settingsMode(cwd string, env []string) string {
	for _, layer := range h.settingsLayers(cwd, env) {
		settings := &claudeSettings{}
		if err := dd.BindJSONFile(settings, layer.path); err != nil {
			if !errors.Is(err, os.ErrNotExist) {
				dl.Warnf("reading claude settings '%s': %v", layer.path, err)
			}
			continue
		}
		mode := settings.Permissions.DefaultMode
		if mode == "" || (layer.project && ignoredInProject[mode]) {
			continue
		}
		return mode
	}
	return ""
}

// Settings says what the operator's claude settings name for a session in
// cwd: the first model, effortLevel and permissions.defaultMode, each from
// the first layer, in claude's precedence, that names it. per-model effort
// overrides (modelSettings) and environment variables are not read, so this
// is what the settings files say at the top level. a new conversation copies
// it once (service.copyFor), which checks each value against what mega
// offers. it only reads.
func (h *Harness) Settings(_ context.Context, place harness.Place) (model.TurnSettings, error) {
	var settings model.TurnSettings
	for _, layer := range h.settingsLayers(place.Cwd, place.Env) {
		file := &claudeSettings{}
		if err := dd.BindJSONFile(file, layer.path); err != nil {
			continue
		}
		if settings.Model == "" {
			settings.Model = file.Model
		}
		if settings.Effort == "" {
			settings.Effort = file.EffortLevel
		}
	}
	settings.Mode = h.settingsMode(place.Cwd, place.Env)
	return settings, nil
}

func (h *Harness) settingsLayers(cwd string, env []string) []settingsLayer {
	managed := h.cfg.ManagedSettings
	if managed == "" {
		managed = DefaultManagedSettings
	}
	layers := []settingsLayer{{path: managed}}
	if dir := localSettingsDir(cwd); dir != cwd {
		layers = append(layers, settingsLayer{path: filepath.Join(dir, ".claude", "settings.local.json"), project: true})
	}
	layers = append(layers,
		settingsLayer{path: filepath.Join(cwd, ".claude", "settings.local.json"), project: true},
		settingsLayer{path: filepath.Join(cwd, ".claude", "settings.json"), project: true},
	)
	// the claude a launch starts reads its user settings where the project's
	// environment points it, so this does too.
	if dir, ok := lookup(env, "CLAUDE_CONFIG_DIR"); ok && dir != "" {
		return append(layers, settingsLayer{path: filepath.Join(dir, "settings.json")})
	}
	home, ok := lookup(env, "HOME")
	if !ok || home == "" {
		home, _ = os.UserHomeDir()
	}
	if home != "" {
		layers = append(layers, settingsLayer{path: filepath.Join(home, ".claude", "settings.json")})
	}
	return layers
}

// localSettingsDir is where claude keeps settings.local.json for a session in
// cwd: the git repository's root, unless cwd is outside a repository, the root
// is the home directory, the checkout is a worktree (claude uses the main
// checkout's root, which is not resolved here), or the root, its .git or its
// .claude is not owned by this user. in each of those cases it stays in cwd.
func localSettingsDir(cwd string) string {
	start, err := filepath.EvalSymlinks(cwd)
	if err != nil {
		return cwd
	}
	for dir := start; ; dir = filepath.Dir(dir) {
		gitInfo, err := os.Lstat(filepath.Join(dir, ".git"))
		if err == nil {
			if !gitInfo.IsDir() {
				return cwd // a worktree
			}
			if home, err := os.UserHomeDir(); err == nil {
				if realHome, err := filepath.EvalSymlinks(home); err == nil && realHome == dir {
					return cwd
				}
			}
			for _, entry := range []string{dir, filepath.Join(dir, ".git"), filepath.Join(dir, ".claude")} {
				if !ownedByMe(entry) {
					return cwd
				}
			}
			if dir == start {
				return cwd
			}
			return dir
		}
		if dir == filepath.Dir(dir) {
			return cwd
		}
	}
}

// ownedByMe reports whether path belongs to this user; a path that does not
// exist counts as owned.
func ownedByMe(path string) bool {
	info, err := os.Lstat(path)
	if err != nil {
		return errors.Is(err, os.ErrNotExist)
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	return !ok || int(stat.Uid) == os.Getuid()
}

// lookup reads a variable from an environment list.
func lookup(env []string, name string) (string, bool) {
	for _, kv := range env {
		if n, v, ok := strings.Cut(kv, "="); ok && n == name {
			return v, true
		}
	}
	return "", false
}
