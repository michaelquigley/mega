package main

import (
	_ "embed"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/spf13/cobra"
)

// the desktop id is the reverse-DNS app id the tray uses, so the desktop ties
// the launcher, its icon and the tray together.
const (
	desktopAppId   = appID
	desktopAppName = "mega"
)

// the hub, rendered from ui/public/favicon.svg at the hicolor sizes by
// render-icons.mjs: committed PNGs, embedded so a plain go build carries them.
//
//go:embed desktop-icon-32.png
var desktopIcon32 []byte

//go:embed desktop-icon-48.png
var desktopIcon48 []byte

//go:embed desktop-icon-192.png
var desktopIcon192 []byte

//go:embed desktop-icon-512.png
var desktopIcon512 []byte

// desktopIconSizes lists the square pixel sizes available from
// desktopIconPNG, ascending.
func desktopIconSizes() []int {
	return []int{32, 48, 192, 512}
}

// desktopIconPNG returns the mark as PNG bytes at the given square
// pixel size; nil outside desktopIconSizes.
func desktopIconPNG(size int) []byte {
	switch size {
	case 32:
		return desktopIcon32
	case 48:
		return desktopIcon48
	case 192:
		return desktopIcon192
	case 512:
		return desktopIcon512
	default:
		return nil
	}
}

func newDesktopCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "desktop",
		Short: "Manage the Linux desktop launcher for mega",
	}
	cmd.AddCommand(newDesktopIntegrateCmd(), newDesktopRemoveCmd())
	return cmd
}

func newDesktopIntegrateCmd() *cobra.Command {
	var plain bool
	cmd := &cobra.Command{
		Use:   "integrate",
		Short: "Install mega's desktop launcher and icons (it runs `mega daemon` through your shell)",
		Args:  cobra.NoArgs,
	}
	cmd.Flags().BoolVar(&plain, "plain", false, "start mega directly, with the desktop session's environment, not through your shell")
	cmd.RunE = func(cmd *cobra.Command, _ []string) error {
		if runtime.GOOS != "linux" {
			return fmt.Errorf("desktop integration is only supported on linux (got '%s')", runtime.GOOS)
		}

		executable, err := os.Executable()
		if err != nil {
			return fmt.Errorf("resolve executable: %w", err)
		}
		executable, err = filepath.Abs(executable)
		if err != nil {
			return fmt.Errorf("resolve executable path: %w", err)
		}

		dataHome, err := desktopDataHome()
		if err != nil {
			return err
		}
		paths := newDesktopInstallPaths(dataHome)
		launch := desktopLaunch(executable, os.Getenv("SHELL"), plain)
		if err := installDesktopFiles(paths, launch.exec); err != nil {
			return err
		}

		out := cmd.OutOrStdout()
		fmt.Fprintf(out, "installed desktop entry: %s\n", paths.desktopFile)
		for _, icon := range paths.icons {
			fmt.Fprintf(out, "installed icon: %s\n", icon.path)
		}
		if launch.warning != "" {
			fmt.Fprintf(out, "warning: %s\n", launch.warning)
		}
		fmt.Fprintf(out, "Exec=%s\n%s\n", launch.exec, launch.why)
		return nil
	}
	return cmd
}

func newDesktopRemoveCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "remove",
		Short: "Remove the desktop launcher and icons that integrate installed",
		Args:  cobra.NoArgs,
	}
	cmd.RunE = func(cmd *cobra.Command, _ []string) error {
		dataHome, err := desktopDataHome()
		if err != nil {
			return err
		}
		paths := newDesktopInstallPaths(dataHome)

		out := cmd.OutOrStdout()
		for _, file := range paths.allFiles() {
			if err := os.Remove(file); err != nil {
				if errors.Is(err, os.ErrNotExist) {
					continue
				}
				return fmt.Errorf("removing '%s': %w", file, err)
			}
			fmt.Fprintf(out, "removed: %s\n", file)
		}
		return nil
	}
	return cmd
}

type desktopIcon struct {
	size int
	path string
}

type desktopInstallPaths struct {
	desktopFile string
	icons       []desktopIcon
}

func (p desktopInstallPaths) allFiles() []string {
	files := make([]string, 0, len(p.icons)+1)
	files = append(files, p.desktopFile)
	for _, icon := range p.icons {
		files = append(files, icon.path)
	}
	return files
}

func desktopDataHome() (string, error) {
	return desktopDataHomeFromEnv(os.Getenv("XDG_DATA_HOME"), os.Getenv("HOME"))
}

func desktopDataHomeFromEnv(xdgDataHome, home string) (string, error) {
	if filepath.IsAbs(xdgDataHome) {
		return xdgDataHome, nil
	}
	if home == "" {
		return "", errors.New("HOME is required when XDG_DATA_HOME is unset or relative")
	}
	return filepath.Join(home, ".local", "share"), nil
}

func newDesktopInstallPaths(dataHome string) desktopInstallPaths {
	paths := desktopInstallPaths{
		desktopFile: filepath.Join(dataHome, "applications", desktopAppId+".desktop"),
	}
	for _, size := range desktopIconSizes() {
		dir := fmt.Sprintf("%dx%d", size, size)
		paths.icons = append(paths.icons, desktopIcon{
			size: size,
			path: filepath.Join(dataHome, "icons", "hicolor", dir, "apps", desktopAppId+".png"),
		})
	}
	return paths
}

func installDesktopFiles(paths desktopInstallPaths, execLine string) error {
	dirs := []string{filepath.Dir(paths.desktopFile)}
	for _, icon := range paths.icons {
		dirs = append(dirs, filepath.Dir(icon.path))
	}
	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return fmt.Errorf("creating desktop install directory '%s': %w", dir, err)
		}
	}

	if err := os.WriteFile(paths.desktopFile, []byte(desktopEntry(execLine)), 0o644); err != nil {
		return fmt.Errorf("write desktop entry: %w", err)
	}
	for _, icon := range paths.icons {
		data := desktopIconPNG(icon.size)
		if len(data) == 0 {
			return fmt.Errorf("missing embedded icon for size %d", icon.size)
		}
		if err := os.WriteFile(icon.path, data, 0o644); err != nil {
			return fmt.Errorf("write %dpx icon: %w", icon.size, err)
		}
	}
	return nil
}

// desktopEntry builds the FreeDesktop entry around its Exec line, which runs
// the daemon. there is no StartupWMClass: the page opens in the default
// browser, so there is no mega-owned window to match. no autostart entry is
// written; mega starts when the operator starts it.
func desktopEntry(execLine string) string {
	return strings.Join([]string{
		"[Desktop Entry]",
		"Type=Application",
		"Name=" + desktopAppName,
		"Comment=" + tagline,
		"Exec=" + execLine,
		"Icon=" + desktopAppId,
		"Terminal=false",
		"Categories=Utility;Development;",
		"",
	}, "\n")
}

// launchForm is how the launcher starts mega: its Exec line, why, and a
// warning when it had to fall back to the plain form.
type launchForm struct {
	exec    string
	why     string
	warning string
}

// the shells the launcher knows how to run interactively.
var launchShells = map[string]bool{"bash": true, "zsh": true, "fish": true}

// desktopLaunch builds the launcher's Exec line. a desktop launcher starts
// programs with the desktop session's environment, which lacks whatever the
// operator's shell start-up files add (a PATH with node and the toolchains
// his agents run). so the launcher runs the operator's shell, named by
// SHELL, as an interactive shell that execs mega: mega then has the
// environment a new terminal has, and hands it to every agent. plain, or a
// shell it does not know, keeps the direct form.
func desktopLaunch(executable, shell string, plain bool) launchForm {
	direct := desktopExecPath(executable) + " daemon"
	if plain {
		return launchForm{exec: direct, why: "the launcher starts mega directly, with the desktop session's environment (--plain)."}
	}
	name := filepath.Base(shell)
	if !filepath.IsAbs(shell) || !launchShells[name] {
		what := fmt.Sprintf("SHELL is '%s', which mega does not know how to start interactively", shell)
		if shell == "" {
			what = "SHELL is not set"
		}
		return launchForm{
			exec:    direct,
			why:     "the launcher starts mega directly, with the desktop session's environment.",
			warning: what + "; the launcher starts mega directly, so mega and its agents get the desktop session's PATH, not your shell's. set SHELL to the absolute path of bash, zsh or fish and run integrate again, or pass --plain to keep this form.",
		}
	}
	script := "exec " + shellQuote(name, executable) + " daemon"
	return launchForm{
		exec: desktopExecPath(shell) + " -i -c " + desktopExecPath(script),
		why:  fmt.Sprintf("the launcher starts mega through your interactive shell, '%s', so mega and every agent it runs get the PATH and environment a new terminal has, not the desktop session's.", name),
	}
}

// shellQuote quotes path for the shell's -c script: single quotes, with an
// embedded quote written as POSIX shells and fish each read it.
func shellQuote(shell, path string) string {
	if shell == "fish" {
		return "'" + strings.NewReplacer(`\`, `\\`, `'`, `\'`).Replace(path) + "'"
	}
	return "'" + strings.ReplaceAll(path, "'", `'\''`) + "'"
}

func desktopExecPath(path string) string {
	replacer := strings.NewReplacer(
		"\\", "\\\\",
		"\"", "\\\"",
		"`", "\\`",
		"$", "\\$",
		"%", "%%",
	)
	return `"` + replacer.Replace(path) + `"`
}
