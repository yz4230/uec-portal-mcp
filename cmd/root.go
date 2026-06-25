package cmd

import (
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"github.com/lmittmann/tint"
	"github.com/spf13/cobra"
)

var rootPstFlags struct {
	verbose bool
}

var rootCmd = &cobra.Command{
	Use: filepath.Base(os.Args[0]),
	PersistentPreRun: func(cmd *cobra.Command, args []string) {
		options := &tint.Options{AddSource: true, TimeFormat: time.TimeOnly}
		if rootPstFlags.verbose {
			options.Level = slog.LevelDebug
		}
		logger := slog.New(tint.NewHandler(os.Stderr, options))
		slog.SetDefault(logger)
	},
}

func Execute() {
	err := rootCmd.Execute()
	if err != nil {
		os.Exit(1)
	}
}

func init() {
	rootCmd.PersistentFlags().BoolVarP(&rootPstFlags.verbose, "verbose", "v", false, "Enable verbose output")
}
