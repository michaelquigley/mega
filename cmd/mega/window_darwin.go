//go:build darwin

package main

import (
	"context"
	"image"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/michaelquigley/df/dl"
	"github.com/michaelquigley/dfw/tray"
	"github.com/michaelquigley/dfw/webview"
	"github.com/spf13/cobra"
)

// on macOS mega has a window of its own, dfw's webview, and the tray offers
// the browser beside it.
const (
	nativeWindow = true
	browserItems = 1
)

const windowTitle = "mega"

var windowCmd = &cobra.Command{
	Use:   "window",
	Short: "Open mega in its own window",
	Long: "Open a running mega in its own window. The daemon starts this for its tray; on its own it " +
		"opens the daemon this user runs, or the mega at the address in DFW_DAEMON_ADDR.",
	Args: cobra.NoArgs,
	RunE: func(_ *cobra.Command, _ []string) error {
		return webview.Window(webview.WindowApp{
			AppID:       appID,
			Title:       windowTitle,
			InitialSize: image.Pt(1280, 860),
		})
	},
}

func init() {
	rootCmd.AddCommand(windowCmd)
}

// openWindow starts mega's window on the mega at addr, as a process of its
// own, and does not wait for it: the window outlives a launch that hands over
// to a running mega, and a daemon that opens one is not held up by it.
func openWindow(addr string) error {
	return tray.SpawnSelf("window")(addr)
}

// setDefaultLang gives mega a LANG from the macOS locale when it has none,
// as an application launchd starts never does. a project's shell is resolved
// from mega's environment, so its tools get the operator's UTF-8 locale and
// not the C one.
func setDefaultLang() {
	if os.Getenv("LANG") != "" || os.Getenv("LC_ALL") != "" {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, "/usr/bin/defaults", "read", "-g", "AppleLocale").Output()
	if err != nil {
		dl.Warnf("LANG is unset and the macOS locale could not be read (%v); using '%s'", err, fallbackLang)
	}
	lang := langFor(strings.TrimSpace(string(out)), func(lang string) bool {
		info, err := os.Stat(filepath.Join("/usr/share/locale", lang))
		return err == nil && info.IsDir()
	})
	if err := os.Setenv("LANG", lang); err != nil {
		dl.Warnf("setting LANG to '%s': %v", lang, err)
		return
	}
	dl.Infof("LANG was unset, as it is for an application; set it to '%s' from the macOS locale", lang)
}
