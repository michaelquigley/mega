//go:build !darwin

package main

import "errors"

// off macOS the browser is mega's only window: no window command, and no
// second tray item for the browser.
const (
	nativeWindow = false
	browserItems = 0
)

func openWindow(string) error {
	return errors.New("mega has no window of its own here; it opens in the browser")
}

func setDefaultLang() {}
