package procs

import (
	"fmt"
	"seer/pkg/proc"
	"strconv"

	"github.com/spf13/cobra"
)

func ProcsDescribe() *cobra.Command {
	describe := &cobra.Command{
		Use:   "describe [pid ...]",
		Short: "Describe processes",
		Long:  `Describe information about a process or processes.`,
		Run: func(cmd *cobra.Command, args []string) {
			if len(args) == 0 {
				procs := proc.GetProcesses()
				for _, p := range procs {
					fmt.Print(p.Describe())
				}
			} else {
				for _, a := range args {
					pid, err := strconv.Atoi(a)
					if err != nil {
						fmt.Printf("Failed to convert '%s' to int.\n\n", a)
						cmd.Help()
						return
					}
					p, err := proc.GetProcess(pid)
					if err != nil {
						fmt.Printf("Warning: %s\n", err)
						continue
					}
					fmt.Print(p.Describe())
				}
			}
		},
	}

	return describe
}
