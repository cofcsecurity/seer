package procs

import (
	"fmt"
	"seer/pkg/proc"
	"sort"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
)

func printTree(root int, procs_map map[int]proc.Process) {
	out := make([]string, 0)
	if r, exists := procs_map[root]; exists {
		out = append(out, fmt.Sprintf("[%d] %s %s\n", r.Pid, r.Exelink, r.Cmdline))
	} else if !exists && root != 0 {
		fmt.Printf("The process '%d' does not exist.\n", root)
		return
	}

	sorted_procs := make([]proc.Process, 0)
	for _, p := range procs_map {
		sorted_procs = append(sorted_procs, p)
	}
	sort.Slice(sorted_procs, func(i, j int) bool { return sorted_procs[i].Pid < sorted_procs[j].Pid })

	var dfs func(start int, level int, procs []proc.Process)
	dfs = func(start int, level int, procs []proc.Process) {
		for _, p := range procs {
			if p.Ppid == start {
				line := strings.Repeat("─", level)
				line += fmt.Sprintf("[%d] %s %s\n", p.Pid, p.Exelink, p.Cmdline)
				out = append(out, line)
				dfs(p.Pid, level+1, procs)
			}
		}
	}

	dfs(root, 0, sorted_procs)
	for i, l := range out {
		if i == 0 {
			if len(out) > 1 {
				fmt.Print("┌")
			}
		} else if i == len(out)-1 {
			fmt.Print("└")
		} else {
			fmt.Print("├")
		}
		fmt.Print(l)
	}
}

func ProcsTree() *cobra.Command {
	tree := &cobra.Command{
		Use:   "tree [pid]",
		Short: "Display a process tree",
		Args:  cobra.MaximumNArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			root := 0
			var err error
			if len(args) == 1 {
				root, err = strconv.Atoi(args[0])
				if err != nil {
					fmt.Printf("Unable to convert '%s' to int.\n\n", args[0])
					cmd.Help()
					return
				}
			}
			proc_map := proc.GetProcesses()
			printTree(root, proc_map)
		},
	}

	return tree
}
