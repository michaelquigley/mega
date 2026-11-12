package main

import (
	"github.com/michaelquigley/df/dl"
	"github.com/spf13/cobra"
)

const trimPrefix = "github.com/michaelquigley/"

var (
	verbose    bool
	configPath string
	dataPath   string
)

var rootCmd = &cobra.Command{
	Use:           "mega",
	Short:         "Project-oriented workspace for a gang of coding agents",
	Args:          cobra.NoArgs,
	SilenceUsage:  true,
	SilenceErrors: true,
	RunE:          runServe,
}

func init() {
	dl.Init(dl.DefaultOptions().SetTrimPrefix(trimPrefix))
	rootCmd.PersistentFlags().BoolVarP(&verbose, "verbose", "v", false, "Enable verbose output")
	rootCmd.PersistentFlags().StringVar(&configPath, "config", "", "Config file path (default $XDG_CONFIG_HOME/mega/mega.yaml)")
	rootCmd.PersistentFlags().StringVar(&dataPath, "data", "", "Data directory (default $XDG_DATA_HOME/mega)")
	rootCmd.PersistentPreRun = func(cmd *cobra.Command, _ []string) {
		// the commands that serve log to a file when no terminal sees them.
		switch cmd {
		case rootCmd, serveCmd, daemonCmd:
			initLogging(true)
		default:
			initLogging(false)
		}
	}
}

func init() {
	rootCmd.AddCommand(newDesktopCmd())
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		dl.Fatalf("%v", err)
	}
}
