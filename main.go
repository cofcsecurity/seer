package main

import (
	"os"
	"seer/cmd/groups"
	"seer/cmd/procs"
	"seer/cmd/socks"
	"seer/cmd/users"

	"log/slog"

	"github.com/spf13/cobra"
)

func main() {
	var logLevel = new(slog.LevelVar)
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: logLevel}))
	slog.SetDefault(logger)

	var verboseLogging bool

	root := &cobra.Command{
		Use:   "seer",
		Short: "Seer is a system enumeration and administration tool for linux",
		PersistentPreRun: func(cmd *cobra.Command, args []string) {
			if verboseLogging {
				logLevel.Set(slog.LevelDebug)
			}
		},
	}

	root.AddCommand(users.Users())
	root.AddCommand(groups.Groups())
	root.AddCommand(procs.Procs())
	root.AddCommand(socks.Socks())

	root.PersistentFlags().BoolVarP(&verboseLogging, "verbose", "v", false, "enable verbose logging")

	root.Execute()
}
