package main

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/michaelquigley/df/dl"
	"golang.org/x/sys/unix"
)

// logFileName is the log under the state directory, used when mega serves
// with no terminal to write to, as when the desktop launcher starts the
// daemon.
const logFileName = "mega.log"

// logPath is where mega logs with no terminal: $XDG_STATE_HOME/mega/mega.log
// for an absolute XDG_STATE_HOME, else ~/.local/state/mega/mega.log.
func logPath() (string, error) {
	return logPathFromEnv(os.Getenv("XDG_STATE_HOME"), os.Getenv("HOME"))
}

func logPathFromEnv(xdgStateHome, home string) (string, error) {
	if filepath.IsAbs(xdgStateHome) {
		return filepath.Join(xdgStateHome, "mega", logFileName), nil
	}
	if home == "" {
		return "", errors.New("HOME is required when XDG_STATE_HOME is unset or relative")
	}
	return filepath.Join(home, ".local", "state", "mega", logFileName), nil
}

// onTerminal is whether f is a terminal. a character device is not enough:
// /dev/null is one, and a launcher often hands a daemon exactly that.
func onTerminal(f *os.File) bool {
	_, err := unix.IoctlGetTermios(int(f.Fd()), unix.TCGETS)
	return err == nil
}

// initLogging sets dl up for the command about to run: the level the flags
// ask for, and, for a command that serves (serve, daemon, or the bare
// default) with no terminal on stdout, output appended to the log file
// rather than lost. dl writes JSON lines when not on a terminal.
func initLogging(serving bool) {
	options := dl.DefaultOptions().SetTrimPrefix(trimPrefix)
	if verbose {
		options.SetLevel(slog.LevelDebug)
	}
	if serving && !onTerminal(os.Stdout) {
		if f, path, err := openLog(); err != nil {
			fmt.Fprintf(os.Stderr, "mega: logging to stdout, since the log file cannot be opened: %v\n", err)
		} else {
			// dl decides json and colour from stdout, which can look like a
			// terminal when it is /dev/null; the file always takes json lines.
			options.SetOutput(f)
			options.UseJSON = true
			options.UseColor = false
			defer dl.Infof("logging to '%s'", path)
		}
	}
	dl.Init(options)
}

// logLimit is the size past which the log is moved aside at start-up.
const logLimit = 10 << 20

// capLog moves a log larger than limit to <path>.1, replacing an earlier one,
// so the next open starts a fresh file. it runs only at start-up: a running
// mega's log grows until the next start.
func capLog(path string, limit int64) error {
	info, err := os.Stat(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}
	if info.Size() <= limit {
		return nil
	}
	return os.Rename(path, path+".1")
}

// openLog opens the log file for appending, making its directory private.
func openLog() (*os.File, string, error) {
	path, err := logPath()
	if err != nil {
		return nil, "", err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, "", err
	}
	if err := capLog(path, logLimit); err != nil {
		fmt.Fprintf(os.Stderr, "mega: the log could not be moved aside: %v\n", err)
	}
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0o600)
	if err != nil {
		return nil, "", err
	}
	return f, path, nil
}
