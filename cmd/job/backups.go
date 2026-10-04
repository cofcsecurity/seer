package job

import (
	"fmt"
	"strings"
	"time"

	"seer/pkg/job"

	"github.com/spf13/cobra"
)

func JobBackups() *cobra.Command {
	return &cobra.Command{
		Use: "backups", Short: "List backups saved before cron files were changed",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			backups, warnings := job.Backups()
			for _, b := range backups {
				fmt.Fprint(cmd.OutOrStdout(), backupLine(b))
			}
			if len(backups) == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "No backups.")
			}
			for _, w := range warnings {
				fmt.Fprintln(cmd.ErrOrStderr(), "Warning: "+w)
			}
			return nil
		},
	}
}

func backupLine(b job.Backup) string {
	what := b.Op + " " + b.Source
	if b.Source == "" {
		what = b.Path + " (old naming; source unknown)"
	}
	line := fmt.Sprintf("[%s] %s (%s ago) %s\n", b.ID, b.Time.Format("2006-01-02 15:04"), age(time.Since(b.Time)), what)
	if summary := b.Summary(); summary != "" {
		line += "    " + summary + "\n"
	}
	return line
}

func age(d time.Duration) string {
	switch {
	case d < time.Hour:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	case d < 48*time.Hour:
		return fmt.Sprintf("%dh", int(d.Hours()))
	default:
		return fmt.Sprintf("%dd", int(d.Hours()/24))
	}
}

func completeBackupIDs(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	if len(args) != 0 {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	backups, _ := job.Backups()
	var out []string
	for _, b := range backups {
		if b.Source != "" && strings.HasPrefix(b.ID, toComplete) {
			out = append(out, b.ID+"\t"+b.Op+" "+b.Source)
		}
	}
	return out, cobra.ShellCompDirectiveNoFileComp
}
