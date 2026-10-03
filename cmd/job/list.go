package job

import (
	"fmt"

	"seer/pkg/job"

	"github.com/spf13/cobra"
)

func JobList() *cobra.Command {
	var userFilter string
	var onlyDisabled, onlyEnabled, showLegend, onlyRisky bool
	list := &cobra.Command{
		Use: "list", Aliases: []string{"ls"},
		Short: "List scheduled cron jobs across all users",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			jobs, warnings := job.ListJobs()
			color := terminalColor(cmd.OutOrStdout())
			for _, j := range jobs {
				if (userFilter != "" && j.User != userFilter) ||
					(onlyRisky && len(j.Risks) == 0) || (onlyDisabled && j.Enabled) || (onlyEnabled && !j.Enabled) {
					continue
				}
				fmt.Fprint(cmd.OutOrStdout(), jobOutput(j, false, color))
			}
			if showLegend {
				fmt.Fprint(cmd.OutOrStdout(), legend(color))
			}
			errColor := terminalColor(cmd.ErrOrStderr())
			for _, w := range warnings {
				fmt.Fprintln(cmd.ErrOrStderr(), paint("Warning: "+w, ansiYellow, errColor))
			}
			return nil
		},
	}
	list.Flags().StringVarP(&userFilter, "user", "u", "", "only show jobs owned by this user")
	list.Flags().BoolVar(&onlyDisabled, "disabled", false, "only show disabled jobs")
	list.Flags().BoolVar(&onlyEnabled, "enabled", false, "only show enabled jobs")
	list.Flags().BoolVar(&onlyRisky, "risky", false, "only show root jobs whose command could be tampered with")
	list.Flags().BoolVar(&showLegend, "legend", false, "print a legend explaining colors and tags")
	list.MarkFlagsMutuallyExclusive("disabled", "enabled")
	return list
}
