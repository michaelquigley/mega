package main

import (
	"bytes"
	"fmt"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDesktopDataHomeFromEnv(t *testing.T) {
	if got, err := desktopDataHomeFromEnv("/tmp/mega-data", "/home/example"); err != nil || got != "/tmp/mega-data" {
		t.Errorf("an absolute XDG_DATA_HOME wins: %q, %v", got, err)
	}
	want := filepath.Join("/home/example", ".local", "share")
	for _, xdg := range []string{"", "relative-data"} {
		if got, err := desktopDataHomeFromEnv(xdg, "/home/example"); err != nil || got != want {
			t.Errorf("XDG_DATA_HOME %q falls back to home: %q, %v", xdg, got, err)
		}
	}
	if _, err := desktopDataHomeFromEnv("", ""); err == nil {
		t.Error("the fallback needs HOME")
	}
}

func TestDesktopEntry(t *testing.T) {
	entry := desktopEntry(`/opt/mega tools/mega`)
	for _, want := range []string{
		"[Desktop Entry]\n",
		"Type=Application\n",
		"Name=mega\n",
		"Comment=a workspace for a gang of coding agents\n",
		`Exec="/opt/mega tools/mega" daemon` + "\n",
		"Icon=com.michaelquigley.mega\n",
		"Terminal=false\n",
		"Categories=Utility;Development;\n",
	} {
		if !strings.Contains(entry, want) {
			t.Errorf("entry lacks %q:\n%s", want, entry)
		}
	}
	// the browser is the window, so there is no mega window class to match.
	if strings.Contains(entry, "StartupWMClass") {
		t.Errorf("entry carries StartupWMClass:\n%s", entry)
	}
	if got := desktopExecPath("/tmp/$HOME/`cmd`%"); got != "\"/tmp/\\$HOME/\\`cmd\\`%%\"" {
		t.Errorf("exec path escaping: %s", got)
	}
}

// integrate writes exactly the entry and the four icons under the data home,
// twice without complaint; remove deletes exactly those, twice without
// complaint, and nothing else; each says what it did. no autostart entry is
// ever written.
func TestIntegrateAndRemoveTwice(t *testing.T) {
	dataHome := t.TempDir()
	t.Setenv("XDG_DATA_HOME", dataHome)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(dataHome, "config"))
	neighbour := filepath.Join(dataHome, "applications", "other.desktop")
	if err := os.MkdirAll(filepath.Dir(neighbour), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(neighbour, []byte("[Desktop Entry]\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	paths := newDesktopInstallPaths(dataHome)
	if want := filepath.Join(dataHome, "applications", "com.michaelquigley.mega.desktop"); paths.desktopFile != want {
		t.Fatalf("desktop file = %q", paths.desktopFile)
	}

	run := func(args ...string) string {
		t.Helper()
		var out bytes.Buffer
		cmd := newDesktopCmd()
		cmd.SetOut(&out)
		cmd.SetArgs(args)
		if err := cmd.Execute(); err != nil {
			t.Fatal(err)
		}
		return out.String()
	}
	for i := range 2 {
		out := run("integrate")
		if !strings.Contains(out, "installed desktop entry: "+paths.desktopFile) || strings.Count(out, "installed icon: ") != 4 {
			t.Errorf("integrate %d says:\n%s", i+1, out)
		}
	}
	entry, err := os.ReadFile(paths.desktopFile)
	if err != nil {
		t.Fatal(err)
	}
	exe, _ := os.Executable()
	exe, _ = filepath.Abs(exe)
	if !strings.Contains(string(entry), "Exec="+desktopExecPath(exe)+" daemon\n") {
		t.Errorf("entry does not run this executable's daemon:\n%s", entry)
	}
	for _, icon := range paths.icons {
		want := filepath.Join(dataHome, "icons", "hicolor", fmt.Sprintf("%dx%d", icon.size, icon.size), "apps", "com.michaelquigley.mega.png")
		if icon.path != want {
			t.Errorf("icon path = %q, want %q", icon.path, want)
		}
		data, err := os.ReadFile(icon.path)
		if err != nil {
			t.Fatal(err)
		}
		cfg, err := png.DecodeConfig(bytes.NewReader(data))
		if err != nil || cfg.Width != icon.size || cfg.Height != icon.size {
			t.Errorf("%d px icon: %dx%d, %v", icon.size, cfg.Width, cfg.Height, err)
		}
	}
	var autostart []string
	_ = filepath.Walk(dataHome, func(path string, _ os.FileInfo, _ error) error {
		if strings.Contains(path, "autostart") {
			autostart = append(autostart, path)
		}
		return nil
	})
	if len(autostart) != 0 {
		t.Errorf("an autostart entry was written: %v", autostart)
	}

	first := run("remove")
	if strings.Count(first, "removed: ") != 5 {
		t.Errorf("remove says:\n%s", first)
	}
	if second := run("remove"); second != "" {
		t.Errorf("a second remove says:\n%s", second)
	}
	for _, file := range paths.allFiles() {
		if _, err := os.Stat(file); !os.IsNotExist(err) {
			t.Errorf("%s survives removal", file)
		}
	}
	if _, err := os.Stat(neighbour); err != nil {
		t.Errorf("remove touched another entry: %v", err)
	}
}
