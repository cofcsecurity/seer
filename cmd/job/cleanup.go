package job

import (
	"fmt"
	"time"

	"seer/pkg/job"
	"seer/pkg/utils"

	"github.com/spf13/cobra"
)

func JobCleanup() *cobra.Command {
	var yes, all bool
	var olderThan string
	cleanup := &cobra.Command{
		Use: "cleanup", Short: "Delete old backups saved by job changes",
		Long: "Delete backups saved before cron files were changed. By default only backups older\n" +
			"than 30 days are removed; use --older-than to change that or --all to remove every\n" +
			"backup. Deleted backups can no longer be restored.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			out := cmd.OutOrStdout()

			var minAge time.Duration
			if !all {
				var err error
				if minAge, err = job.ParseAge(olderThan); err != nil {
					return err
				}
			}

			backups, warnings := job.Backups()
			for _, w := range warnings {
				fmt.Fprintln(cmd.ErrOrStderr(), "Warning: "+w)
			}
			doomed := job.SelectBackups(backups, minAge, time.Now())
			if len(doomed) == 0 {
				fmt.Fprintln(out, "No backups to delete.")
				return nil
			}

			for _, b := range doomed {
				fmt.Fprint(out, backupLine(b))
			}
			fmt.Fprintf(out, "%d backup(s) will be deleted and can no longer be restored.\n", len(doomed))
			if !yes && !utils.Confirm() {
				fmt.Fprintln(out, "Canceled.")
				return nil
			}
			if all && !yes {
				fmt.Fprintln(out, "This removes every backup, including the only copy of any removed job.")
				if !utils.Confirm() {
					fmt.Fprintln(out, "Canceled.")
					return nil
				}
			}

			removed, err := job.RemoveBackups(doomed)
			fmt.Fprintf(out, "Deleted %d backup(s).\n", removed)
			return err
		},
	}
	cleanup.Flags().StringVar(&olderThan, "older-than", "30d", "only delete backups older than this (for example 12h, 30d, 2w)")
	cleanup.Flags().BoolVar(&all, "all", false, "delete every backup regardless of age")
	cleanup.Flags().BoolVarP(&yes, "yes", "y", false, "skip confirmation")
	return cleanup
}
