package main

import (
	"fmt"

	"github.com/michaelquigley/push/build"
	"github.com/spf13/cobra"
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
