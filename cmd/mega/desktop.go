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
	cmd := &cobra.Command{
		Use:   "integrate",
		Short: "Install mega's desktop launcher and icons (it runs `mega daemon`)",
		Args:  cobra.NoArgs,
	}
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
		if err := installDesktopFiles(paths, executable); err != nil {
			return err
		}

		out := cmd.OutOrStdout()
		fmt.Fprintf(out, "installed desktop entry: %s\n", paths.desktopFile)
		for _, icon := range paths.icons {
			fmt.Fprintf(out, "installed icon: %s\n", icon.path)
		}
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

func installDesktopFiles(paths desktopInstallPaths, executable string) error {
	dirs := []string{filepath.Dir(paths.desktopFile)}
	for _, icon := range paths.icons {
		dirs = append(dirs, filepath.Dir(icon.path))
	}
	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return fmt.Errorf("creating desktop install directory '%s': %w", dir, err)
		}
	}

	if err := os.WriteFile(paths.desktopFile, []byte(desktopEntry(executable)), 0o644); err != nil {
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

// desktopEntry builds the FreeDesktop entry: it runs the daemon. there is no
// StartupWMClass: the page opens in the default browser, so there is no
// mega-owned window to match. no autostart entry is written; mega starts when
// the operator starts it.
func desktopEntry(executable string) string {
	return strings.Join([]string{
		"[Desktop Entry]",
		"Type=Application",
		"Name=" + desktopAppName,
		"Comment=a workspace for a gang of coding agents",
		"Exec=" + desktopExecPath(executable) + " daemon",
		"Icon=" + desktopAppId,
		"Terminal=false",
		"Categories=Utility;Development;",
		"",
	}, "\n")
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
