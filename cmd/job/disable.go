package job

import (
	"fmt"

	"seer/pkg/job"
	"seer/pkg/utils"

	"github.com/spf13/cobra"
)

func JobDisable() *cobra.Command {
	var yes, preview bool
	disable := &cobra.Command{
		Use: "disable [job-id]", Short: "Disable a scheduled cron job",
		Args:              cobra.ExactArgs(1),
		ValidArgsFunction: completeJobIDs(func(j job.Job) bool { return !j.ReadOnly && j.Enabled }),
		RunE: func(cmd *cobra.Command, args []string) error {
			target, err := findTarget(args[0])
			if err != nil {
				return err
			}
			if preview {
				text, err := job.Diff(job.OpDisable, args[0])
				if err != nil {
					return err
				}
				fmt.Fprint(cmd.OutOrStdout(), colorDiff(text, terminalColor(cmd.OutOrStdout())))
				return nil
			}
			color := terminalColor(cmd.OutOrStdout())
			if color {
				fmt.Fprint(cmd.OutOrStdout(), legend(true))
			}
			fmt.Fprint(cmd.OutOrStdout(), jobOutput(*target, true, color))
			if !yes && !utils.Confirm() {
				fmt.Fprintln(cmd.OutOrStdout(), "Canceled.")
				return nil
			}
			backup, err := job.Disable(args[0])
			if err != nil {
				return err
			}
			fmt.Fprintln(cmd.OutOrStdout(), paint("Job disabled. Backup saved to "+backup, ansiCyan, terminalColor(cmd.OutOrStdout())))
			return nil
		},
	}
	disable.Flags().BoolVarP(&yes, "yes", "y", false, "skip confirmation")
	disable.Flags().BoolVar(&preview, "preview", false, "show only the lines that would change, without applying them")
	return disable
}

func findTarget(id string) (*job.Job, error) {
	jobs, _ := job.ListJobs()
	for _, j := range jobs {
		if j.ID == id {
			if j.ReadOnly {
				return nil, fmt.Errorf("job %q comes from %s and is read-only; edit %s directly", id, j.Kind, j.Source)
			}
			return &j, nil
		}
	}
	return nil, fmt.Errorf("job %q is no longer visible", id)
}
