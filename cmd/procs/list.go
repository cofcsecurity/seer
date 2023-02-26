package procs

import (
	"fmt"
	"seer/pkg/proc"

	"github.com/spf13/cobra"
)

// Print process info with procs grouped by executable
func groupByExe(procs []proc.Process) {
	exes := make(map[string][]proc.Process)
	for _, p := range procs {
		exe := p.Exelink
		// If exelink is empty fall back to comm (kernel threads)
		if exe == "" {
			exe = fmt.Sprintf("(%s)", p.Comm)
		}
		exes[exe] = append(exes[exe], p)
	}
	for e := range exes {
		fmt.Printf("┌<%s> (Count: %d)\n", e, len(exes[e]))
		for i, p := range exes[e] {
			line := '├'
			if i == len(exes[e])-1 {
				line = '└'
			}
			fmt.Printf("%c[%d]->[%d] %s started %d seconds ago by %s\n", line, p.Ppid, p.Pid, p.Cmdline, p.Age(), p.User.Username)
		}
	}
}

func ProcsList() *cobra.Command {
	var byExe bool

	list := &cobra.Command{
		Use:     "list",
		Aliases: []string{"ls"},
		Short:   "List running processes",
		Run: func(cmd *cobra.Command, args []string) {
			procs := proc.GetProcesses()

			if byExe {
				groupByExe(procs)
			} else {
				for _, p := range procs {
					fmt.Print(p.String())
				}
			}
		},
	}

	list.Flags().BoolVarP(&byExe, "exe", "e", false, "group processes by executable")

	return list
}
