// cmd/job/describe.go
package job

import (
	"fmt"

	"seer/pkg/job"

	"github.com/spf13/cobra"
)

func JobDescribe() *cobra.Command {
	return &cobra.Command{
		Use: "describe [job-id]", Short: "Describe a scheduled cron job",
		Args:              cobra.ExactArgs(1),
		ValidArgsFunction: completeJobIDs(nil),
		RunE: func(cmd *cobra.Command, args []string) error {
			jobs, _ := job.ListJobs()
			for _, j := range jobs {
				if j.ID == args[0] {
					fmt.Fprint(cmd.OutOrStdout(), jobOutput(j, true, terminalColor(cmd.OutOrStdout())))
					return nil
				}
			}
			return fmt.Errorf("job %q is no longer visible", args[0])
		},
	}
}
