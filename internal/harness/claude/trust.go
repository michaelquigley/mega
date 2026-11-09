// Package claude is the Claude Code adapter.
package claude

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/michaelquigley/df/dd"
)

type claudeConfig struct {
	Projects map[string]claudeProject `dd:"projects"`
}

type claudeProject struct {
	HasTrustDialogAccepted bool `dd:"hasTrustDialogAccepted"`
}

// DefaultConfigPath is where the operator's claude keeps its trust state.
func DefaultConfigPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolving home directory: %w", err)
	}
	return filepath.Join(home, ".claude.json"), nil
}

// CheckoutTrusted reports whether claude will honor the checkout's project
// permission rules. it only ever reads the config: trust is granted by the
// operator opening the checkout interactively, never by this service. a missing
// config means nothing has been trusted yet.
func CheckoutTrusted(configPath, checkout string) (bool, error) {
	cfg := &claudeConfig{}
	if err := dd.BindJSONFile(cfg, configPath); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return false, nil
		}
		return false, fmt.Errorf("reading '%s': %w", configPath, err)
	}
	return cfg.Projects[trustKey(checkout)].HasTrustDialogAccepted, nil
}

// trustKey mirrors how claude 2.1.318 keys the trust check that gates permission
// rules: the enclosing git root, else the real path. it is not a documented
// contract, and a linked worktree may read as untrusted; the notice is
// informational, so that errs toward prompting a harmless terminal visit.
func trustKey(checkout string) string {
	realPath, err := filepath.EvalSymlinks(checkout)
	if err != nil {
		realPath = filepath.Clean(checkout)
	}
	for dir := realPath; ; dir = filepath.Dir(dir) {
		if _, err := os.Lstat(filepath.Join(dir, ".git")); err == nil {
			return dir
		}
		if dir == filepath.Dir(dir) {
			return realPath
		}
	}
}
