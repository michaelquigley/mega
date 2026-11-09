package main

import (
	"log/slog"

	"github.com/michaelquigley/df/dl"
	"github.com/spf13/cobra"
)

const trimPrefix = "git.hq.quigley.com/research/"

var (
	verbose    bool
	configPath string
	dataPath   string
)

var rootCmd = &cobra.Command{
	Use:           "roundhouse",
	Short:         "Project-oriented workspace for a gang of coding agents",
	Args:          cobra.NoArgs,
	SilenceUsage:  true,
	SilenceErrors: true,
	RunE:          runServe,
}

func init() {
	dl.Init(dl.DefaultOptions().SetTrimPrefix(trimPrefix))
	rootCmd.PersistentFlags().BoolVarP(&verbose, "verbose", "v", false, "Enable verbose output")
	rootCmd.PersistentFlags().StringVar(&configPath, "config", "", "Config file path (default $XDG_CONFIG_HOME/roundhouse/roundhouse.yaml)")
	rootCmd.PersistentFlags().StringVar(&dataPath, "data", "", "Data directory (default $XDG_DATA_HOME/roundhouse)")
	rootCmd.PersistentPreRun = func(_ *cobra.Command, _ []string) {
		if verbose {
			dl.Init(dl.DefaultOptions().SetTrimPrefix(trimPrefix).SetLevel(slog.LevelDebug))
		}
	}
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		dl.Fatalf("%v", err)
	}
}
