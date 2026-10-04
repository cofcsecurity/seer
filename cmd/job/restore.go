package job

import (
	"fmt"

	"seer/pkg/job"
	"seer/pkg/utils"

	"github.com/spf13/cobra"
)

func JobRestore() *cobra.Command {
	var yes, preview, wholeFile bool
	restore := &cobra.Command{
		Use: "restore [backup-id]", Short: "Put a cron file back to a saved backup",
		Long: "Undo a change listed by 'seer job backups'. Only the line that was disabled,\n" +
			"enabled, or removed is put back; other edits made since are left alone. Use\n" +
			"--whole-file to replace the whole cron file with the saved copy instead, which\n" +
			"also reverts those other edits. If the file was deleted it is recreated. The\n" +
			"current contents are saved as a new backup, so a restore can itself be undone.",
		Args:              cobra.ExactArgs(1),
		ValidArgsFunction: completeBackupIDs,
		RunE: func(cmd *cobra.Command, args []string) error {
			out := cmd.OutOrStdout()
			color := terminalColor(out)

			summary, short, err := job.RestoreSummary(args[0], wholeFile)
			if err != nil {
				return err
			}
			if short && !preview {
				fmt.Fprint(out, summary)
			} else {
				text, err := job.RestoreDiff(args[0], wholeFile)
				if err != nil {
					return err
				}
				fmt.Fprint(out, colorDiff(text, color))
			}
			if preview {
				return nil
			}
			if !yes && !utils.Confirm() {
				fmt.Fprintln(out, "Canceled.")
				return nil
			}

			undo, err := job.Restore(args[0], wholeFile)
			if err != nil {
				return err
			}
			if undo == "" {
				fmt.Fprintln(out, paint("File recreated.", ansiCyan, color))
				return nil
			}
			fmt.Fprintln(out, paint("Restored. Undo with: seer job restore "+job.BackupID(undo), ansiCyan, color))
			return nil
		},
	}
	restore.Flags().BoolVarP(&yes, "yes", "y", false, "skip confirmation")
	restore.Flags().BoolVar(&wholeFile, "whole-file", false, "replace the whole cron file with the backup, reverting other edits too")
	restore.Flags().BoolVar(&preview, "preview", false, "show only the lines that would change, without applying them")
	return restore
}
