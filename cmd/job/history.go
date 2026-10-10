package job

import (
	"fmt"

	"seer/pkg/job"

	"github.com/spf13/cobra"
)

func JobHistory() *cobra.Command {
	var limit int
	history := &cobra.Command{
		Use: "history [job-id]", Short: "Show recent runs of a job from the system logs",
		Args:              cobra.ExactArgs(1),
		ValidArgsFunction: completeJobIDs(nil),
		RunE: func(cmd *cobra.Command, args []string) error {
			target, err := findAny(args[0])
			if err != nil {
				return err
			}

			h, err := job.JobHistory(*target, limit)
			if err != nil {
				return err
			}

			out := cmd.OutOrStdout()
			if len(h.Entries) == 0 {
				fmt.Fprintf(out, "No runs found in %s.\n", h.Source)
			} else {
				fmt.Fprintf(out, "Recent runs from %s:\n", h.Source)
				for _, e := range h.Entries {
					fmt.Fprintln(out, e)
				}
			}
			if h.Note != "" {
				fmt.Fprintf(out, "Note: %s.\n", h.Note)
			}
			return nil
		},
	}
	history.Flags().IntVarP(&limit, "limit", "n", 10, "number of runs to show")
	return history
}

// findAny looks up a job of any kind, including read-only ones.
func findAny(id string) (*job.Job, error) {
	jobs, _ := job.ListJobs()
	for _, j := range jobs {
		if j.ID == id {
			return &j, nil
		}
	}
	return nil, fmt.Errorf("job %q is no longer visible", id)
}
