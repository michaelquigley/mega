package main

import (
	"fmt"
	"runtime"

	"github.com/michaelquigley/push/build"
	"github.com/spf13/cobra"

	"github.com/michaelquigley/mega/internal/harness/claude"
	"github.com/michaelquigley/mega/internal/server"
	"github.com/michaelquigley/mega/ui"
)

func init() {
	rootCmd.AddCommand(versionCmd())
}

// versionCmd is push's version command, followed by where mega logs when it
// serves with no terminal, so the operator can find it.
func versionCmd() *cobra.Command {
	cmd := build.NewVersionCmd("mega")
	run := cmd.Run
	cmd.Run = func(c *cobra.Command, args []string) {
		run(c, args)
		if path, err := logPath(); err == nil {
			fmt.Printf("log file, when serving with no terminal: %s\n", path)
		}
	}
	return cmd
}

// about is what the service answers on /api/version: what 'mega version'
// prints, from the same build information, with where it logs, the Claude
// Agent SDK beside the shim, and the identity of the interface it serves.
func about(claudeHarness *claude.Harness) server.About {
	a := server.About{
		Version:    build.Version,
		Developer:  build.Version == "" || build.Hash == "",
		Go:         runtime.Version(),
		Target:     runtime.GOOS + "/" + runtime.GOARCH,
		LogsToFile: logsToFile,
		Interface:  ui.Build(),
	}
	if a.Developer {
		a.Version = build.DevVersion
	} else {
		a.Commit, a.Built, a.Branch = build.Hash, build.Date, build.Branch
	}
	if build.CGO == "1" {
		a.Go += " (cgo)"
	}
	if path, err := logPath(); err == nil {
		a.LogFile = path
	} else {
		a.LogFile = fmt.Sprintf("unknown: %v", err)
	}
	if version, err := claudeHarness.SDKVersion(); err == nil {
		a.ClaudeSDK = version
	} else {
		a.ClaudeSDK = fmt.Sprintf("unknown: %v", err)
	}
	return a
}
