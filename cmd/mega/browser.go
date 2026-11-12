package main

import (
	"fmt"
	"os/exec"
	"runtime"
	"time"
)

// browserArgv builds the platform's url opener invocation.
func browserArgv(goos, url string) ([]string, error) {
	switch goos {
	case "linux":
		return []string{"xdg-open", url}, nil
	default:
		return nil, fmt.Errorf("no browser opener for '%s'; browse to '%s' by hand", goos, url)
	}
}

// openBrowser points the operator's default browser at url and gets out of
// the way: the browser is mega's window, and it does the rest.
func openBrowser(url string) error {
	argv, err := browserArgv(runtime.GOOS, url)
	if err != nil {
		return err
	}
	cmd := exec.Command(argv[0], argv[1:]...)
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("running '%s': %w", argv[0], err)
	}
	// reap the opener, so a long-lived daemon gathers no zombies.
	go func() { _ = cmd.Wait() }()
	return nil
}

// handOverWait bounds how long a launch that hands over waits for the opener
// before it exits: long enough for xdg-open to pass the url on, never long
// enough for a browser that keeps the opener waiting to hold the launch open.
const handOverWait = 2 * time.Second

// openBrowserAndWait is openBrowser for a process about to exit: it waits
// for the opener to finish, up to wait, so exiting cannot cut it short.
func openBrowserAndWait(url string, wait time.Duration) error {
	argv, err := browserArgv(runtime.GOOS, url)
	if err != nil {
		return err
	}
	cmd := exec.Command(argv[0], argv[1:]...)
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("running '%s': %w", argv[0], err)
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case err := <-done:
		if err != nil {
			return fmt.Errorf("running '%s': %w", argv[0], err)
		}
	case <-time.After(wait):
	}
	return nil
}
