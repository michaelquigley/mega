package config

import (
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"

	"github.com/michaelquigley/df/dd"
)

const (
	appName         = "roundhouse"
	configFileName  = appName + ".yaml"
	registryFile    = "registry.yaml"
	lockFile        = "lock"
	conversationDir = "conversations"
	defaultListen   = "127.0.0.1:8470"

	// the claude code build the shim's sdk (0.3.318) was matched against.
	defaultClaudeExecutable = "~/.local/share/claude/versions/2.1.318"
)

// defaultClaudeShim is the shim directory, stamped at build time by the
// Makefile (-ldflags -X) because the shim and its node_modules live in the
// checkout, not in the binary.
var defaultClaudeShim string

// Config is the environment service's configuration.
type Config struct {
	Listen string
	Data   string
	Claude Claude
}

// Claude locates what the claude adapter runs: node, the shim directory, and
// the claude code executable the sdk drives.
type Claude struct {
	Node       string
	Shim       string
	Executable string
}

// Load resolves config from compiled defaults, then the config file, then the
// data flag. an empty configPath means the XDG default, which may be absent;
// an explicit configPath must exist.
func Load(configPath, dataOverride string) (*Config, error) {
	cfg := &Config{
		Listen: defaultListen,
		Claude: Claude{Node: "node", Shim: defaultClaudeShim, Executable: defaultClaudeExecutable},
	}
	if configPath != "" {
		if err := dd.MergeYAMLFile(cfg, configPath); err != nil {
			return nil, err
		}
	} else {
		defaultPath, err := defaultConfigPath()
		if err != nil {
			return nil, err
		}
		if err := dd.MergeYAMLFile(cfg, defaultPath); err != nil && !errors.Is(err, os.ErrNotExist) {
			return nil, err
		}
	}
	if dataOverride != "" {
		cfg.Data = dataOverride
	}
	if cfg.Data == "" {
		defaultData, err := defaultDataDir()
		if err != nil {
			return nil, err
		}
		cfg.Data = defaultData
	}
	for _, path := range []*string{&cfg.Data, &cfg.Claude.Shim, &cfg.Claude.Executable} {
		expanded, err := expandPath(*path)
		if err != nil {
			return nil, err
		}
		*path = expanded
	}
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}
	return cfg, nil
}

// Validate enforces that the listener stays on loopback. this slice has no
// login, so an address reachable from the network would expose agents with
// filesystem and command capabilities.
func (c *Config) Validate() error {
	host, _, err := net.SplitHostPort(c.Listen)
	if err != nil {
		return fmt.Errorf("listen '%s': %w", c.Listen, err)
	}
	if host == "localhost" {
		return nil
	}
	if ip := net.ParseIP(host); ip == nil || !ip.IsLoopback() {
		return fmt.Errorf("listen '%s' is not a loopback address", c.Listen)
	}
	return nil
}

// RegistryPath is the authority file for projects and conversations.
func (c *Config) RegistryPath() string {
	return filepath.Join(c.Data, registryFile)
}

// LockPath is the file whose exclusive hold makes one service the data
// directory's only writer.
func (c *Config) LockPath() string {
	return filepath.Join(c.Data, lockFile)
}

// EventLogPath is the append-only event history of one conversation.
func (c *Config) EventLogPath(conversationID string) string {
	return filepath.Join(c.Data, conversationDir, conversationID, "events.jsonl")
}

func defaultConfigPath() (string, error) {
	base, err := xdgDir("XDG_CONFIG_HOME", ".config")
	if err != nil {
		return "", err
	}
	return filepath.Join(base, appName, configFileName), nil
}

func defaultDataDir() (string, error) {
	base, err := xdgDir("XDG_DATA_HOME", filepath.Join(".local", "share"))
	if err != nil {
		return "", err
	}
	return filepath.Join(base, appName), nil
}

func xdgDir(env, homeRelative string) (string, error) {
	if dir := os.Getenv(env); dir != "" {
		return dir, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolving home directory: %w", err)
	}
	return filepath.Join(home, homeRelative), nil
}

func expandPath(path string) (string, error) {
	if path != "~" && !strings.HasPrefix(path, "~/") {
		return path, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolving home directory: %w", err)
	}
	return filepath.Join(home, strings.TrimPrefix(path, "~")), nil
}
