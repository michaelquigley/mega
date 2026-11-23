package main

import (
	"fmt"
	"net/url"
	"strings"
)

// appLaunchEnv is set to "1" by the macOS application's Info.plist
// (LSEnvironment, in macos/Info.plist), so a launch from Finder, the Dock or
// open runs the daemon, as the Linux launcher does, rather than serve.
const appLaunchEnv = "MEGA_APP"

// launchArgs is the command line mega runs, and whether this is a launch of
// the application: the daemon for a bare launch of the application, the
// arguments as given for anything else, including the application's own
// binary run from a terminal.
func launchArgs(args []string, getenv func(string) string) ([]string, bool) {
	if len(args) == 0 && getenv(appLaunchEnv) == "1" {
		return []string{"daemon"}, true
	}
	return args, false
}

// launchedAsApp is whether this process is a launch of the application: its
// daemon opens mega's window as soon as it serves, so a launch from Finder
// shows something beside the menu bar icon.
var launchedAsApp bool

// hostOf is the address in a running mega's url, as a window is pointed at
// it: "127.0.0.1:6342" for "http://127.0.0.1:6342".
func hostOf(raw string) (string, error) {
	u, err := url.Parse(raw)
	if err != nil {
		return "", fmt.Errorf("reading the address in '%s': %w", raw, err)
	}
	if u.Host == "" {
		return "", fmt.Errorf("'%s' names no address", raw)
	}
	return u.Host, nil
}

// fallbackLang is the LANG for a macOS locale with no UTF-8 locale of its
// own installed; every macOS has it.
const fallbackLang = "en_US.UTF-8"

// langFor is the LANG for a macOS AppleLocale such as "sv_SE", "en_US" or
// "en_SE@rg=sezzzz": the language and region before any '@', in UTF-8, when
// the system has that locale (installed reports it), else fallbackLang.
// launchd starts an application with no LANG, and a project's shell resolved
// from it would otherwise run its tools in the C locale.
func langFor(appleLocale string, installed func(lang string) bool) string {
	name, _, _ := strings.Cut(strings.TrimSpace(appleLocale), "@")
	if name == "" {
		return fallbackLang
	}
	lang := name + ".UTF-8"
	if !installed(lang) {
		return fallbackLang
	}
	return lang
}
