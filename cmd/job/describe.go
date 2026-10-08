// cmd/job/describe.go
package job

import (
	"fmt"

	"seer/pkg/job"

	"github.com/spf13/cobra"
)

func JobDescribe() *cobra.Command {
	var asJSON bool
	describe := &cobra.Command{
		Use: "describe [job-id]", Short: "Describe a scheduled cron job",
		Args:              cobra.ExactArgs(1),
		ValidArgsFunction: completeJobIDs(nil),
		RunE: func(cmd *cobra.Command, args []string) error {
			jobs, _ := job.ListJobs()
			for _, j := range jobs {
				if j.ID == args[0] {
					if asJSON {
						return printJSON(cmd, j, nil)
					}
					color := terminalColor(cmd.OutOrStdout())
					if color {
						fmt.Fprint(cmd.OutOrStdout(), legend(true))
					}
					fmt.Fprint(cmd.OutOrStdout(), jobOutput(j, true, color))
					return nil
				}
			}
			return fmt.Errorf("job %q is no longer visible", args[0])
		},
	}
	describe.Flags().BoolVar(&asJSON, "json", false, "print the job as JSON")
	return describe
}
