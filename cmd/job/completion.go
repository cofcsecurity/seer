package job

import (
	"fmt"
	"strings"

	"seer/pkg/job"

	"github.com/spf13/cobra"
)

func completeJobIDs(filter func(job.Job) bool) func(*cobra.Command, []string, string) ([]string, cobra.ShellCompDirective) {
	return func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		if len(args) != 0 {
			return nil, cobra.ShellCompDirectiveNoFileComp
		}
		jobs, _ := job.ListJobs()
		return jobIDCompletions(jobs, filter, toComplete), cobra.ShellCompDirectiveNoFileComp
	}
}

func jobIDCompletions(jobs []job.Job, filter func(job.Job) bool, prefix string) []string {
	var matches []string
	for _, j := range jobs {
		if (filter != nil && !filter(j)) || !strings.HasPrefix(j.ID, prefix) {
			continue
		}
		description := fmt.Sprintf("%s %s", j.Schedule, j.Command)
		if !j.Enabled {
			description += " DISABLED"
		}
		matches = append(matches, j.ID+"\t"+description)
	}
	return matches
}
