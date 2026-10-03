package job

import (
	"encoding/json"
	"fmt"

	"seer/pkg/job"

	"github.com/spf13/cobra"
)

func JobList() *cobra.Command {
	var userFilter string
	var onlyDisabled, onlyEnabled, onlyRisky, asJSON bool
	list := &cobra.Command{
		Use: "list", Aliases: []string{"ls"},
		Short: "List scheduled cron jobs across all users",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			jobs, warnings := job.ListJobs()
			color := terminalColor(cmd.OutOrStdout())
			if asJSON {
				return printJSON(cmd, filterJobs(jobs, userFilter, onlyDisabled, onlyEnabled, onlyRisky), warnings)
			}
			shown := filterJobs(jobs, userFilter, onlyDisabled, onlyEnabled, onlyRisky)
			// The color key is only useful when something is colored.
			if color && len(shown) > 0 {
				fmt.Fprint(cmd.OutOrStdout(), legend(true))
			}
			for _, j := range shown {
				fmt.Fprint(cmd.OutOrStdout(), jobOutput(j, false, color))
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
	list.Flags().BoolVar(&asJSON, "json", false, "print jobs as JSON")
	list.MarkFlagsMutuallyExclusive("disabled", "enabled")
	return list
}

func filterJobs(jobs []job.Job, user string, disabled, enabled, risky bool) []job.Job {
	out := make([]job.Job, 0, len(jobs))
	for _, j := range jobs {
		if (user != "" && j.User != user) || (risky && len(j.Risks) == 0) ||
			(disabled && j.Enabled) || (enabled && !j.Enabled) {
			continue
		}
		out = append(out, j)
	}
	return out
}

// printJSON writes value as indented JSON; warnings still go to stderr.
func printJSON(cmd *cobra.Command, value any, warnings []string) error {
	enc := json.NewEncoder(cmd.OutOrStdout())
	enc.SetIndent("", "  ")
	for _, w := range warnings {
		fmt.Fprintln(cmd.ErrOrStderr(), "Warning: "+w)
	}
	return enc.Encode(value)
}
