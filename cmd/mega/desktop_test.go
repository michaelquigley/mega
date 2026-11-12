package main

import (
	"bytes"
	"fmt"
	"image/png"
	"os"
	"os/exec"
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
	entry := desktopEntry(desktopLaunch(`/opt/mega tools/mega`, "", true).exec)
	for _, want := range []string{
		"[Desktop Entry]\n",
		"Type=Application\n",
		"Name=mega\n",
		"Comment=" + tagline + "\n",
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

	t.Setenv("SHELL", "/bin/bash")
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
	if want := desktopLaunch(exe, "/bin/bash", false).exec; !strings.Contains(string(entry), "Exec="+want+"\n") {
		t.Errorf("entry does not run this executable's daemon through bash:\n%s", entry)
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

// the launcher runs the operator's interactive shell, which execs the daemon;
// --plain, and a shell it does not know, keep the direct form.
func TestDesktopLaunchForms(t *testing.T) {
	exe := "/opt/mega/mega"
	for shell, want := range map[string]string{
		"/bin/bash":           `"/bin/bash" -i -c "exec '/opt/mega/mega' daemon"`,
		"/usr/bin/zsh":        `"/usr/bin/zsh" -i -c "exec '/opt/mega/mega' daemon"`,
		"/usr/local/bin/fish": `"/usr/local/bin/fish" -i -c "exec '/opt/mega/mega' daemon"`,
	} {
		got := desktopLaunch(exe, shell, false)
		if got.exec != want || got.warning != "" || !strings.Contains(got.why, "interactive shell, '"+filepath.Base(shell)+"',") {
			t.Errorf("%s: %+v, want %s", shell, got, want)
		}
	}
	plain := desktopLaunch(exe, "/bin/bash", true)
	if plain.exec != `"/opt/mega/mega" daemon` || plain.warning != "" || !strings.Contains(plain.why, "--plain") {
		t.Errorf("--plain: %+v", plain)
	}
	for _, shell := range []string{"", "/bin/tcsh", "bash"} {
		got := desktopLaunch(exe, shell, false)
		if got.exec != `"/opt/mega/mega" daemon` || got.warning == "" {
			t.Errorf("SHELL %q falls back with a warning: %+v", shell, got)
		}
	}
	// a path the shell must quote: a space and a single quote.
	odd := "/opt/it's mega/mega"
	if got := desktopLaunch(odd, "/bin/bash", false).exec; got != `"/bin/bash" -i -c "exec '/opt/it'\\''s mega/mega' daemon"` {
		t.Errorf("bash quoting: %s", got)
	}
	if got := desktopLaunch(odd, "/usr/bin/fish", false).exec; got != `"/usr/bin/fish" -i -c "exec '/opt/it\\'s mega/mega' daemon"` {
		t.Errorf("fish quoting: %s", got)
	}
}

// execArgs splits an Exec value as the desktop entry specification reads it:
// arguments separated by spaces, double-quoted ones with \\, \", \` and \$
// escapes, and %% for a literal %.
func execArgs(t *testing.T, exec string) []string {
	t.Helper()
	var args []string
	var cur strings.Builder
	quoted, have := false, false
	for i := 0; i < len(exec); i++ {
		c := exec[i]
		switch {
		case quoted && c == '\\' && i+1 < len(exec):
			i++
			cur.WriteByte(exec[i])
		case c == '"':
			quoted, have = !quoted, true
		case !quoted && c == ' ':
			if have {
				args = append(args, cur.String())
				cur.Reset()
				have = false
			}
		case c == '%' && i+1 < len(exec) && exec[i+1] == '%':
			i++
			cur.WriteByte('%')
			have = true
		default:
			cur.WriteByte(c)
			have = true
		}
	}
	if quoted {
		t.Fatalf("unterminated quote in %s", exec)
	}
	if have {
		args = append(args, cur.String())
	}
	return args
}

// the generated line, read as a desktop reads it and run by each shell that is
// installed, starts the binary with daemon, even from a path with a space and
// a quote. HOME is a temporary directory, so no real start-up file is read.
func TestDesktopLaunchRunsThroughEachShell(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "it's mega")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(t.TempDir(), "args")
	stand := filepath.Join(dir, "mega")
	if err := os.WriteFile(stand, []byte("#!/bin/sh\necho \"$@\" > "+out+"\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	ran := 0
	for _, name := range []string{"bash", "zsh", "fish"} {
		shell, err := exec.LookPath(name)
		if err != nil {
			continue
		}
		ran++
		_ = os.Remove(out)
		line := desktopLaunch(stand, shell, false).exec
		args := execArgs(t, line)
		if len(args) != 4 || args[0] != shell || args[1] != "-i" || args[2] != "-c" {
			t.Fatalf("%s: argv %q from %s", name, args, line)
		}
		cmd := exec.Command(args[0], args[1:]...)
		cmd.Env = append(os.Environ(), "HOME="+t.TempDir(), "XDG_CONFIG_HOME="+t.TempDir())
		if output, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("%s: %v\n%s", name, err, output)
		}
		got, err := os.ReadFile(out)
		if err != nil || strings.TrimSpace(string(got)) != "daemon" {
			t.Errorf("%s: the stand-in got %q, %v", name, got, err)
		}
	}
	if ran == 0 {
		t.Skip("no bash, zsh or fish installed")
	}
}

// desktop-file-validate, where installed, finds no error or warning in any
// form of the entry.
func TestDesktopEntryValidates(t *testing.T) {
	validate, err := exec.LookPath("desktop-file-validate")
	if err != nil {
		t.Skip("desktop-file-validate is not installed")
	}
	for _, launch := range []launchForm{
		desktopLaunch("/opt/mega tools/mega", "/bin/bash", false),
		desktopLaunch("/opt/mega/mega", "/usr/bin/fish", false),
		desktopLaunch("/opt/mega/mega", "/bin/bash", true),
	} {
		file := filepath.Join(t.TempDir(), "com.michaelquigley.mega.desktop")
		if err := os.WriteFile(file, []byte(desktopEntry(launch.exec)), 0o644); err != nil {
			t.Fatal(err)
		}
		// hints are advice (the card's two main categories draw one); errors
		// and warnings are not.
		if output, err := exec.Command(validate, file).CombinedOutput(); err != nil || strings.Contains(string(output), "error:") || strings.Contains(string(output), "warning:") {
			t.Errorf("%s: %v\n%s", launch.exec, err, output)
		}
	}
}
