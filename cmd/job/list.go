package job

import (
	"fmt"

	"seer/pkg/job"

	"github.com/spf13/cobra"
)

func JobList() *cobra.Command {
	return &cobra.Command{
		Use: "list", Aliases: []string{"ls"},
		Short: "List scheduled cron jobs across all users",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			jobs, warnings := job.ListJobs()
			color := terminalColor(cmd.OutOrStdout())
			for _, j := range jobs {
				fmt.Fprint(cmd.OutOrStdout(), jobOutput(j, false, color))
			}
			errColor := terminalColor(cmd.ErrOrStderr())
			for _, w := range warnings {
				fmt.Fprintln(cmd.ErrOrStderr(), paint("Warning: "+w, ansiYellow, errColor))
			}
			return nil
		},
	}
}
