package config

import (
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"unicode"

	"github.com/michaelquigley/df/dd"
)

const (
	appName         = "mega"
	configFileName  = appName + ".yaml"
	registryFile    = "registry.yaml"
	lockFile        = "lock"
	conversationDir = "conversations"
	// 6342, MEGA on a telephone keypad, unassigned by IANA. it was 8470
	// until 2026-11-18, which IANA assigns to another protocol and NetFoundry's
	// Agora also uses.
	defaultListen = "127.0.0.1:6342"

	// the installed claude code, by its bare name: the one the operator's own
	// updater links, so an update never leaves mega on a pruned build. with
	// none on the PATH the adapter falls back to the newest native build in
	// the installer's versions folder. the builds mega was tested with are
	// harness.Tested. exported so the harness round trips (internal/e2e) run
	// what mega runs by default, and cannot drift from it.
	DefaultClaudeExecutable = "claude"
)

// defaultClaudeShim is the shim directory, stamped at build time by the
// Makefile (-ldflags -X) because the shim and its node_modules live in the
// checkout, not in the binary. the macOS application is built unstamped and
// carries the shim itself (bundledClaudeShim).
var defaultClaudeShim string

// bundledClaudeShim is where the macOS application carries the shim, for
// the binary at executable: mega.app/Contents/MacOS/mega has it in
// mega.app/Contents/Resources/shim/claude. empty for a binary that is not
// in an application bundle.
func bundledClaudeShim(executable string) string {
	macOS := filepath.Dir(executable)
	contents := filepath.Dir(macOS)
	if filepath.Base(macOS) != "MacOS" || filepath.Base(contents) != "Contents" {
		return ""
	}
	return filepath.Join(contents, "Resources", "shim", "claude")
}

// claudeShim is the shim directory mega runs by default: the one stamped at
// build time, else the one in the application bundle around the binary when
// it holds the shim, else none, which the adapter reports.
func claudeShim(stamped string, executable func() (string, error)) string {
	if stamped != "" {
		return stamped
	}
	exe, err := executable()
	if err != nil {
		return ""
	}
	// a link to the binary, put on the PATH, still finds the bundle.
	if resolved, err := filepath.EvalSymlinks(exe); err == nil {
		exe = resolved
	}
	dir := bundledClaudeShim(exe)
	if dir == "" {
		return ""
	}
	if _, err := os.Stat(filepath.Join(dir, "shim.mjs")); err != nil {
		return ""
	}
	return dir
}

// Config is the environment service's configuration.
type Config struct {
	Listen string
	Data   string
	// Name is what this mega is called on its pages and in 'mega version',
	// so megas open side by side are not mistaken for each other; empty is
	// the machine's hostname (MegaName).
	Name   string
	Claude Claude
	Codex  Codex
	Pi     Pi
	// Environment is how each project's shell environment is resolved.
	Environment Environment
	// Snapshots bounds the configuration snapshots kept under the data
	// directory.
	Snapshots Snapshots
	// PathSearch names directories beyond the checkout the reply box's @
	// search reaches.
	PathSearch PathSearch `dd:"path_search"`
	// ProjectsRoot is where the register dialog's folder picker starts, and
	// all it lists and completes: the home directory when empty. a path
	// typed in full is registered wherever it is.
	ProjectsRoot string `dd:"projects_root"`

	// File is the config file mega read, or the default it looked for, and
	// FileRead whether there was one to read. neither is configuration: they
	// say where to set a value, in a message.
	File     string `dd:"-"`
	FileRead bool   `dd:"-"`
}

// Environment says how a project's environment is resolved: the shell to run
// (empty is the one SHELL names, else /bin/sh) and how long a resolution may
// take, as a Go duration such as "10s" (empty is ten seconds).
type Environment struct {
	Shell   string
	Timeout string
}

// Snapshots is how many configuration snapshots are kept: Keep ordinary
// ones, taken at start-up when the configuration changed, and
// KeepAtVersion taken when a harness reported a new version, which are kept
// longer. Keep 0 takes none at all.
type Snapshots struct {
	Keep          int
	KeepAtVersion int `dd:"keep_at_version"`
}

// PathSearch is the reply box's @ search beyond the checkout: Directories
// the operator names, such as ~/Sandbox, searched in every project when an
// @ word starts with ~/ or /. none, the default, keeps the search to the
// checkout. each is expanded from ~ and must then be absolute.
type PathSearch struct {
	Directories []string
}

// the snapshot bounds when the config names none. claude code updates every
// day or two, so most starts meet a new version: those snapshots get the
// larger bound, about a month of updates, and the ordinary ones, a start
// whose configuration changed with no new version, ten.
const (
	defaultSnapshotKeep          = 10
	defaultSnapshotKeepAtVersion = 20
)

// Pi locates the pi executable, which the pi adapter runs in RPC mode.
type Pi struct {
	Binary string
}

// Codex locates the codex executable, whose app-server the codex adapter runs.
type Codex struct {
	Binary string
}

// Claude locates what the claude adapter runs: node, the shim directory, and
// the claude code executable the sdk drives.
type Claude struct {
	Node       string
	Shim       string
	Executable string
}

// Load resolves config from compiled defaults, then the config file, then the
// data and listen flags. an empty configPath means the XDG default, which may
// be absent; an explicit configPath must exist. a listen flag is held to
// loopback exactly as the config value is.
func Load(configPath, dataOverride, listenOverride string) (*Config, error) {
	cfg := &Config{
		Listen:    defaultListen,
		Claude:    Claude{Node: "node", Shim: claudeShim(defaultClaudeShim, os.Executable), Executable: DefaultClaudeExecutable},
		Codex:     Codex{Binary: "codex"},
		Pi:        Pi{Binary: "pi"},
		Snapshots: Snapshots{Keep: defaultSnapshotKeep, KeepAtVersion: defaultSnapshotKeepAtVersion},
	}
	if configPath != "" {
		if err := dd.MergeYAMLFile(cfg, configPath); err != nil {
			return nil, err
		}
		cfg.File, cfg.FileRead = configPath, true
	} else {
		defaultPath, err := defaultConfigPath()
		if err != nil {
			return nil, err
		}
		cfg.File = defaultPath
		err = dd.MergeYAMLFile(cfg, defaultPath)
		switch {
		case err == nil:
			cfg.FileRead = true
		case !errors.Is(err, os.ErrNotExist):
			return nil, err
		}
	}
	if abs, err := filepath.Abs(cfg.File); err == nil {
		cfg.File = abs
	}
	if dataOverride != "" {
		cfg.Data = dataOverride
	}
	if listenOverride != "" {
		cfg.Listen = listenOverride
	}
	if cfg.Data == "" {
		defaultData, err := defaultDataDir()
		if err != nil {
			return nil, err
		}
		cfg.Data = defaultData
	}
	paths := []*string{&cfg.Data, &cfg.Claude.Shim, &cfg.Claude.Executable, &cfg.ProjectsRoot}
	for i := range cfg.PathSearch.Directories {
		paths = append(paths, &cfg.PathSearch.Directories[i])
	}
	for _, path := range paths {
		expanded, err := expandPath(*path)
		if err != nil {
			return nil, err
		}
		*path = expanded
	}
	cfg.Name = strings.TrimSpace(cfg.Name)
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}
	return cfg, nil
}

// Where says where a config value is set, for a message that tells the
// operator to set one: the file mega read, or the one it looked for.
func (c *Config) Where() string {
	if c.FileRead {
		return fmt.Sprintf("in '%s', the config file mega read", c.File)
	}
	return fmt.Sprintf("in '%s', a config file mega looked for and did not find, so create it", c.File)
}

// Validate enforces that the listener stays on loopback. this slice has no
// login, so an address reachable from the network would expose agents with
// filesystem and command capabilities.
func (c *Config) Validate() error {
	if c.Snapshots.Keep < 0 || c.Snapshots.KeepAtVersion < 0 {
		return fmt.Errorf("snapshots: keep and keep_at_version are counts, 0 or more")
	}
	if len([]rune(c.Name)) > maxName || strings.ContainsFunc(c.Name, unicode.IsControl) {
		return fmt.Errorf("name: at most %d characters, and none a control character", maxName)
	}
	if c.ProjectsRoot != "" && !filepath.IsAbs(c.ProjectsRoot) {
		return fmt.Errorf("projects_root: '%s' is not absolute; name a directory from / or ~/", c.ProjectsRoot)
	}
	for _, dir := range c.PathSearch.Directories {
		if !filepath.IsAbs(dir) {
			return fmt.Errorf("path_search: '%s' is not absolute; name a directory from / or ~/", dir)
		}
	}
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

// maxName bounds a configured name, which a tab title carries.
const maxName = 64

// Hostname is the machine's hostname, os.Hostname unless a test replaces it.
var Hostname = os.Hostname

// the sources of a mega's name (MegaName).
const (
	NameFromConfig   = "config"
	NameFromHostname = "hostname"
)

// MegaName is what this mega is called, and where the name came from: the
// config's name when set, else the machine's hostname up to its first dot,
// so a fully qualified one reads as the machine. both are empty where the
// hostname cannot be read, and the pages say only "mega".
func (c *Config) MegaName() (name, from string) {
	if c.Name != "" {
		return c.Name, NameFromConfig
	}
	host, err := Hostname()
	if err != nil {
		return "", ""
	}
	host, _, _ = strings.Cut(strings.TrimSpace(host), ".")
	if host == "" {
		return "", ""
	}
	return host, NameFromHostname
}

// SnapshotsPath is where configuration snapshots are kept.
func (c *Config) SnapshotsPath() string {
	return filepath.Join(c.Data, "snapshots")
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
