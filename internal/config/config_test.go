package config

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func isolate(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(root, "config"))
	t.Setenv("XDG_DATA_HOME", filepath.Join(root, "data"))
	return root
}

func TestTheShimInAnApplicationBundle(t *testing.T) {
	root := t.TempDir()
	app := filepath.Join(root, "mega.app", "Contents")
	binary := filepath.Join(app, "MacOS", "mega")
	shim := filepath.Join(app, "Resources", "shim", "claude")
	for _, dir := range []string{filepath.Dir(binary), shim} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(binary, nil, 0o755); err != nil {
		t.Fatal(err)
	}
	exe := func(path string) func() (string, error) { return func() (string, error) { return path, nil } }

	if got := claudeShim("", exe(binary)); got != "" {
		t.Errorf("a bundle with no shim in it: claudeShim = %q, want none", got)
	}
	if err := os.WriteFile(filepath.Join(shim, "shim.mjs"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	resolvedShim, err := filepath.EvalSymlinks(shim)
	if err != nil {
		t.Fatal(err)
	}
	if got := claudeShim("", exe(binary)); got != resolvedShim {
		t.Errorf("the bundle's binary: claudeShim = %q, want %q", got, resolvedShim)
	}
	link := filepath.Join(root, "bin", "mega")
	if err := os.MkdirAll(filepath.Dir(link), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(binary, link); err != nil {
		t.Fatal(err)
	}
	if got := claudeShim("", exe(link)); got != resolvedShim {
		t.Errorf("a link to the bundle's binary: claudeShim = %q, want %q", got, resolvedShim)
	}
	if got := claudeShim("/checkout/shim/claude", exe(binary)); got != "/checkout/shim/claude" {
		t.Errorf("a stamped build: claudeShim = %q, want the stamp", got)
	}
	if got := claudeShim("", exe(filepath.Join(root, "bin", "elsewhere"))); got != "" {
		t.Errorf("a binary outside a bundle: claudeShim = %q, want none", got)
	}
	if got := claudeShim("", func() (string, error) { return "", errors.New("no executable") }); got != "" {
		t.Errorf("no executable: claudeShim = %q, want none", got)
	}
}

func TestDefaultsFollowXDG(t *testing.T) {
	root := isolate(t)
	cfg, err := Load("", "", "")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Listen != "127.0.0.1:6342" {
		t.Errorf("listen = %q", cfg.Listen)
	}
	want := filepath.Join(root, "data", "mega")
	if cfg.Data != want {
		t.Errorf("data = %q, want %q", cfg.Data, want)
	}
	if got := cfg.RegistryPath(); got != filepath.Join(want, "registry.yaml") {
		t.Errorf("registry path = %q", got)
	}
	if got := cfg.EventLogPath("abc"); got != filepath.Join(want, "conversations", "abc", "events.jsonl") {
		t.Errorf("event log path = %q", got)
	}
}

func TestConfigFileThenDataFlag(t *testing.T) {
	root := isolate(t)
	dir := filepath.Join(root, "config", "mega")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	body := "listen: 127.0.0.1:9999\ndata: /from/file\n"
	if err := os.WriteFile(filepath.Join(dir, "mega.yaml"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load("", "", "")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Listen != "127.0.0.1:9999" || cfg.Data != "/from/file" {
		t.Errorf("file not applied: %+v", cfg)
	}
	cfg, err = Load("", "/from/flag", "")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Data != "/from/flag" {
		t.Errorf("flag did not win: %q", cfg.Data)
	}
}

// the listen flag wins over the file, and is held to loopback as the file's
// value is.
func TestListenFlagWinsAndStaysOnLoopback(t *testing.T) {
	root := isolate(t)
	dir := filepath.Join(root, "config", "mega")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "mega.yaml"), []byte("listen: 127.0.0.1:9999\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load("", "", "127.0.0.1:18471")
	if err != nil || cfg.Listen != "127.0.0.1:18471" {
		t.Fatalf("listen = %+v, %v", cfg, err)
	}
	for _, addr := range []string{"0.0.0.0:18471", ":18471", "192.168.1.5:80"} {
		if _, err := Load("", "", addr); err == nil || !strings.Contains(err.Error(), "loopback") && !strings.Contains(err.Error(), "listen") {
			t.Errorf("%q: %v", addr, err)
		}
	}
}

// Where names the file mega read, or the one it looked for and did not
// find.
func TestWhereNamesTheConfigFile(t *testing.T) {
	root := isolate(t)
	cfg, err := Load("", "", "")
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(root, "config", "mega", "mega.yaml")
	if cfg.File != want || cfg.FileRead || !strings.Contains(cfg.Where(), want) || !strings.Contains(cfg.Where(), "did not find") {
		t.Errorf("no file: %q %v %q", cfg.File, cfg.FileRead, cfg.Where())
	}
	explicit := filepath.Join(t.TempDir(), "mine.yaml")
	if err := os.WriteFile(explicit, []byte("listen: 127.0.0.1:9999\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err = Load(explicit, "", "")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.File != explicit || !cfg.FileRead || !strings.Contains(cfg.Where(), "the config file mega read") {
		t.Errorf("explicit: %q %v %q", cfg.File, cfg.FileRead, cfg.Where())
	}
}

func TestExplicitConfigMustExist(t *testing.T) {
	isolate(t)
	if _, err := Load(filepath.Join(t.TempDir(), "absent.yaml"), "", ""); err == nil {
		t.Fatal("expected an error for a missing explicit config")
	}
}

func TestTildeExpansion(t *testing.T) {
	isolate(t)
	t.Setenv("HOME", "/home/someone")
	cfg, err := Load("", "~/elsewhere", "")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Data != "/home/someone/elsewhere" {
		t.Errorf("data = %q", cfg.Data)
	}
}

func TestListenMustBeLoopback(t *testing.T) {
	for addr, ok := range map[string]bool{
		"127.0.0.1:8470": true,
		"127.1.2.3:8470": true,
		"[::1]:8470":     true,
		"localhost:8470": true,
		"0.0.0.0:8470":   false,
		":8470":          false,
		"192.168.1.5:80": false,
		"[::]:8470":      false,
		"example.com:80": false,
		"not-an-address": false,
	} {
		err := (&Config{Listen: addr}).Validate()
		if ok && err != nil {
			t.Errorf("%q: unexpected error %v", addr, err)
		}
		if !ok && err == nil {
			t.Errorf("%q: expected an error", addr)
		}
	}
}

func TestClaudeDefaultsMergeWithFile(t *testing.T) {
	root := isolate(t)
	t.Setenv("HOME", root)
	path := filepath.Join(t.TempDir(), "mega.yaml")
	if err := os.WriteFile(path, []byte("claude:\n  shim: ~/shim/claude\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(path, "", "")
	if err != nil {
		t.Fatal(err)
	}
	want := Claude{
		Node:       "node",
		Shim:       filepath.Join(root, "shim", "claude"),
		Executable: "claude",
	}
	if cfg.Claude != want {
		t.Errorf("claude = %+v, want %+v", cfg.Claude, want)
	}
}

// the snapshot bounds default to ten ordinary and twenty taken at a version
// change, are set in the config file, and are counts.
func TestSnapshotBounds(t *testing.T) {
	root := isolate(t)
	cfg, err := Load("", "", "")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Snapshots.Keep != 10 || cfg.Snapshots.KeepAtVersion != 20 {
		t.Errorf("defaults: %+v", cfg.Snapshots)
	}
	file := filepath.Join(root, "mega.yaml")
	for body, want := range map[string]Snapshots{
		"snapshots:\n  keep: 3\n  keep_at_version: 4\n": {Keep: 3, KeepAtVersion: 4},
		"snapshots:\n  keep: 0\n":                       {Keep: 0, KeepAtVersion: 20},
	} {
		if err := os.WriteFile(file, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
		cfg, err := Load(file, "", "")
		if err != nil || cfg.Snapshots != want {
			t.Errorf("%q: %+v, %v", body, cfg, err)
		}
	}
	if err := os.WriteFile(file, []byte("snapshots:\n  keep: -1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(file, "", ""); err == nil || !strings.Contains(err.Error(), "snapshots") {
		t.Errorf("a negative bound: %v", err)
	}
}

// the directories the @ search reaches beyond the checkout: none by default,
// each named from ~ expanded, an absolute one kept, and a relative one
// refused.
func TestPathSearchDirectories(t *testing.T) {
	root := isolate(t)
	t.Setenv("HOME", "/home/someone")
	cfg, err := Load("", "", "")
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.PathSearch.Directories) != 0 {
		t.Errorf("default: %v", cfg.PathSearch.Directories)
	}
	file := filepath.Join(root, "mega.yaml")
	if err := os.WriteFile(file, []byte("path_search:\n  directories:\n    - ~/Sandbox\n    - /srv/shots\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err = Load(file, "", "")
	if err != nil || strings.Join(cfg.PathSearch.Directories, ",") != "/home/someone/Sandbox,/srv/shots" {
		t.Errorf("named: %+v, %v", cfg.PathSearch, err)
	}
	if err := os.WriteFile(file, []byte("path_search:\n  directories:\n    - Sandbox\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(file, "", ""); err == nil || !strings.Contains(err.Error(), "path_search") {
		t.Errorf("a relative directory: %v", err)
	}
}

// a mega is called what the config names it, else the machine's hostname up
// to its first dot; with neither, nothing. a name too long or holding a
// control character is refused. the hostname here is made up, never the
// machine's own.
func TestMegaName(t *testing.T) {
	root := isolate(t)
	host, hostErr := "made-up-host.example.net", error(nil)
	saved := Hostname
	Hostname = func() (string, error) { return host, hostErr }
	t.Cleanup(func() { Hostname = saved })
	cfg, err := Load("", "", "")
	if err != nil {
		t.Fatal(err)
	}
	if name, from := cfg.MegaName(); name != "made-up-host" || from != NameFromHostname {
		t.Errorf("default: %q from %q", name, from)
	}
	file := filepath.Join(root, "mega.yaml")
	if err := os.WriteFile(file, []byte("name: '  desk mega  '\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err = Load(file, "", "")
	if err != nil {
		t.Fatal(err)
	}
	if name, from := cfg.MegaName(); name != "desk mega" || from != NameFromConfig {
		t.Errorf("configured: %q from %q", name, from)
	}
	hostErr = errors.New("no hostname")
	if name, from := (&Config{}).MegaName(); name != "" || from != "" {
		t.Errorf("no hostname: %q from %q", name, from)
	}
	for _, body := range []string{"name: \"a\\tb\"\n", "name: " + strings.Repeat("x", 65) + "\n"} {
		if err := os.WriteFile(file, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
		if _, err := Load(file, "", ""); err == nil || !strings.Contains(err.Error(), "name") {
			t.Errorf("%q: %v", body, err)
		}
	}
}

// the folder picker's root: unset by default, which the service takes as the
// home directory; named from ~ and expanded; a relative one refused.
func TestProjectsRoot(t *testing.T) {
	root := isolate(t)
	t.Setenv("HOME", "/home/someone")
	cfg, err := Load("", "", "")
	if err != nil || cfg.ProjectsRoot != "" {
		t.Fatalf("default: %q, %v", cfg.ProjectsRoot, err)
	}
	file := filepath.Join(root, "mega.yaml")
	if err := os.WriteFile(file, []byte("projects_root: ~/Repos\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if cfg, err := Load(file, "", ""); err != nil || cfg.ProjectsRoot != "/home/someone/Repos" {
		t.Errorf("named: %q, %v", cfg.ProjectsRoot, err)
	}
	if err := os.WriteFile(file, []byte("projects_root: Repos\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(file, "", ""); err == nil || !strings.Contains(err.Error(), "projects_root") {
		t.Errorf("a relative root: %v", err)
	}
}

// the default port is 6342; a config that names a port keeps it, the old
// default 8470 included, and --listen still wins over both.
func TestAConfiguredPortIsKept(t *testing.T) {
	root := isolate(t)
	if cfg, err := Load("", "", ""); err != nil || cfg.Listen != "127.0.0.1:6342" {
		t.Fatalf("the default: %q, %v", cfg.Listen, err)
	}
	file := filepath.Join(root, "mega.yaml")
	if err := os.WriteFile(file, []byte("listen: 127.0.0.1:8470\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if cfg, err := Load(file, "", ""); err != nil || cfg.Listen != "127.0.0.1:8470" {
		t.Errorf("a config naming 8470: %q, %v", cfg.Listen, err)
	}
	if cfg, err := Load(file, "", "127.0.0.1:18471"); err != nil || cfg.Listen != "127.0.0.1:18471" {
		t.Errorf("--listen over the config: %q, %v", cfg.Listen, err)
	}
}
