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
			procs := proc.GetProcesses()
			if len(args) == 0 {
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
					p := -1
					for i := range procs {
						if procs[i].Pid == pid {
							p = i
							break
						}
					}
					if p != -1 {
						fmt.Print(procs[p].Describe())
					} else {
						fmt.Printf("Warning: the process '%d' does not exist\n", pid)
					}
				}
			}
		},
	}

	return describe
}
