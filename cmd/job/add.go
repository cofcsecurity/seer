package job

import (
	"fmt"

	"seer/pkg/job"
	"seer/pkg/utils"

	"github.com/spf13/cobra"
)

func JobAdd() *cobra.Command {
	var spec job.AddSpec
	var yes, preview bool
	add := &cobra.Command{
		Use: "add", Short: "Add a cron job",
		Long: "Add a job to a user's existing crontab, or to a system file with --file (/etc/crontab,\n" +
			"a file in /etc/cron.d, or a user crontab). The file is backed up first, so the job can be\n" +
			"removed again with 'seer job restore'. Escape any % in the command as \\%.",
		Example: "  seer job add --user alice --schedule '30 2 * * 1-5' --command /usr/local/bin/backup.sh\n" +
			"  seer job add --user root --schedule @daily --command /opt/app/clean.sh --file /etc/cron.d/app",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			out := cmd.OutOrStdout()
			color := terminalColor(out)

			plan, err := job.PlanAdd(spec)
			if err != nil {
				return err
			}

			fmt.Fprint(out, colorDiff(job.AddDiff(plan), color))
			for _, risk := range plan.Risks {
				fmt.Fprintln(out, paint("Risk: "+risk, ansiRed, color))
			}
			if preview {
				return nil
			}
			if !yes && !utils.Confirm() {
				fmt.Fprintln(out, "Canceled.")
				return nil
			}

			id, backup, err := job.Add(plan)
			if err != nil {
				return err
			}
			fmt.Fprintln(out, paint("Job added ("+id+"). Undo with: seer job restore "+job.BackupID(backup), ansiCyan, color))
			return nil
		},
	}
	add.Flags().StringVarP(&spec.User, "user", "u", "", "account the job runs as (required)")
	add.Flags().StringVarP(&spec.Schedule, "schedule", "s", "", "cron schedule, such as '30 2 * * 1-5' or @daily (required)")
	add.Flags().StringVarP(&spec.Command, "command", "c", "", "command to run (required)")
	add.Flags().StringVarP(&spec.File, "file", "f", "", "cron file to add the job to (default: the user's crontab)")
	add.Flags().BoolVarP(&yes, "yes", "y", false, "skip confirmation")
	add.Flags().BoolVar(&preview, "preview", false, "show the line that would be added without applying it")
	for _, name := range []string{"user", "schedule", "command"} {
		_ = add.MarkFlagRequired(name)
	}
	return add
}
