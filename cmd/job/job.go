package job

import "github.com/spf13/cobra"

func Job() *cobra.Command {
	job := &cobra.Command{
		Use:   "job",
		Short: "Inspect and manage scheduled jobs across all users via cron",
	}
	job.AddCommand(JobList())
	job.AddCommand(JobDescribe())
	job.AddCommand(JobAdd())
	job.AddCommand(JobDisable())
	job.AddCommand(JobEnable())
	job.AddCommand(JobRemove())
	job.AddCommand(JobCheck())
	job.AddCommand(JobHistory())
	job.AddCommand(JobBackups())
	job.AddCommand(JobRestore())
	job.AddCommand(JobCleanup())
	return job
}
