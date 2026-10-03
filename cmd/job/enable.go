package job

import (
	"fmt"

	"seer/pkg/job"
	"seer/pkg/utils"

	"github.com/spf13/cobra"
)

func JobEnable() *cobra.Command {
	var yes, preview bool
	enable := &cobra.Command{
		Use: "enable [job-id]", Short: "Enable a disabled cron job",
		Args:              cobra.ExactArgs(1),
		ValidArgsFunction: completeJobIDs(func(j job.Job) bool { return !j.ReadOnly && !j.Enabled }),
		RunE: func(cmd *cobra.Command, args []string) error {
			target, err := findTarget(args[0])
			if err != nil {
				return err
			}
			if preview {
				text, err := job.Diff(job.OpEnable, args[0])
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
			backup, err := job.Enable(args[0])
			if err != nil {
				return err
			}
			fmt.Fprintln(cmd.OutOrStdout(), paint("Job enabled. Backup saved to "+backup, ansiCyan, terminalColor(cmd.OutOrStdout())))
			return nil
		},
	}
	enable.Flags().BoolVarP(&yes, "yes", "y", false, "skip confirmation")
	enable.Flags().BoolVar(&preview, "preview", false, "show only the lines that would change, without applying them")
	return enable
}
