package procs

import (
	"fmt"
	"seer/pkg/proc"
	"sort"
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
				pids := []int{}
				for pid := range procs {
					pids = append(pids, pid)
				}
				sort.Slice(pids, func(i, j int) bool { return pids[i] < pids[j] })
				for _, pid := range pids {
					fmt.Print(procs[pid].Describe())
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
