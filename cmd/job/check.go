package job

import (
	"fmt"

	"seer/pkg/job"

	"github.com/spf13/cobra"
)

func JobCheck() *cobra.Command {
	return &cobra.Command{
		Use: "check", Short: "Find duplicate jobs and commands another user can change",
		Long: "Report enabled jobs that overlap: identical jobs, the same command run on different\n" +
			"schedules or by different users, and jobs whose command can be modified by another\n" +
			"account that has its own jobs.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			jobs, warnings := job.ListJobs()
			for _, w := range warnings {
				fmt.Fprintln(cmd.ErrOrStderr(), "Warning: "+w)
			}

			findings := job.Check(jobs)
			if len(findings) == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "No overlapping jobs found.")
				return nil
			}

			titles := map[string]string{
				"duplicate":    "Duplicate jobs",
				"similar":      "Similar jobs",
				"shared-write": "Commands another user can change",
			}
			for _, kind := range []string{"duplicate", "similar", "shared-write"} {
				printed := false
				for _, f := range findings {
					if f.Kind != kind {
						continue
					}
					if !printed {
						fmt.Fprintf(cmd.OutOrStdout(), "%s:\n", titles[kind])
						printed = true
					}
					fmt.Fprintf(cmd.OutOrStdout(), "  %s\n", f.Message)
					for _, id := range f.Jobs {
						fmt.Fprintf(cmd.OutOrStdout(), "    [%s]\n", id)
					}
				}
			}
			return nil
		},
	}
}
