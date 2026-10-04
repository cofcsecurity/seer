package job

import (
	"fmt"

	"seer/pkg/job"
	"seer/pkg/utils"

	"github.com/spf13/cobra"
)

func JobRemove() *cobra.Command {
	var yes, preview bool
	remove := &cobra.Command{
		Use: "remove [job-id]", Short: "Remove a scheduled cron job (undo with job restore)",
		Args:              cobra.ExactArgs(1),
		ValidArgsFunction: completeJobIDs(func(j job.Job) bool { return !j.ReadOnly }),
		RunE: func(cmd *cobra.Command, args []string) error {
			target, err := findTarget(args[0])
			if err != nil {
				return err
			}
			if preview {
				text, err := job.Diff(job.OpRemove, args[0])
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
			fmt.Fprintln(cmd.OutOrStdout(), paint("This deletes the line from its source file. A backup is kept; undo with 'seer job restore'.", ansiYellow, terminalColor(cmd.OutOrStdout())))
			if !yes && !utils.Confirm() {
				fmt.Fprintln(cmd.OutOrStdout(), "Canceled.")
				return nil
			}
			backup, err := job.Remove(args[0])
			if err != nil {
				return err
			}
			fmt.Fprintln(cmd.OutOrStdout(), paint("Job removed. Undo with: seer job restore "+job.BackupID(backup), ansiCyan, terminalColor(cmd.OutOrStdout())))
			return nil
		},
	}
	remove.Flags().BoolVarP(&yes, "yes", "y", false, "skip confirmation")
	remove.Flags().BoolVar(&preview, "preview", false, "show only the lines that would change, without applying them")
	return remove
}
