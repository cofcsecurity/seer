package procs

import (
	"fmt"
	"seer/pkg/proc"
	"sort"

	"github.com/spf13/cobra"
)

// Print process info with procs grouped by executable
func groupByExe(procs map[int]proc.Process) {
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
	var lsFds bool
	var lsSockets bool

	list := &cobra.Command{
		Use:     "list",
		Aliases: []string{"ls"},
		Short:   "List running processes",
		Run: func(cmd *cobra.Command, args []string) {
			procs := proc.GetProcesses()
			pids := []int{}
			for pid := range procs {
				pids = append(pids, pid)
			}
			sort.Slice(pids, func(i, j int) bool { return pids[i] < pids[j] })

			if byExe {
				groupByExe(procs)
			} else {
				i := 0
				for _, pid := range pids {
					p := procs[pid]
					if lsFds || lsSockets {
						// Get fds, these are needed in either case
						fds, e := p.GetFds()
						if e != nil {
							fmt.Printf("Failed to get file descriptors: %s\n", e.Error())
							return
						}

						// Determine the correct edges and print the proc info
						p_edge_0 := "├"
						if i == 0 {
							// If this is the first proc, start the tree
							p_edge_0 = "┬"
						} else if i == len(procs)-1 {
							// If this is the last proc, end the tree
							p_edge_0 = "└"
						}
						p_edge_1 := "─"
						// If this proc will have child items, add an edge for the subtree
						if lsSockets && len(p.Sockets) > 0 {
							p_edge_1 = "┬"
						} else if lsFds && len(fds) > 0 {
							p_edge_1 = "┬"
						}
						fmt.Printf("%s%s%s", p_edge_0, p_edge_1, p.String())

						// Format fds or sockets appropriately
						children := []string{}
						if lsFds {
							for id, fd := range fds {
								children = append(children, fmt.Sprintf("<%d> -> %s\n", id, fd))
							}
						} else if lsSockets {
							for _, s := range p.Sockets {
								children = append(children, s.String())
							}
						}

						// Print the child items of this process
						for j, c := range children {
							s_edge_0 := " "
							if i < len(procs)-1 {
								// If there are more procs left, add an edge for them
								s_edge_0 = "│"
							}
							s_edge_1 := "├"
							if j == len(children)-1 {
								// If this is the last child item end the subtree
								s_edge_1 = "└"
							}
							fmt.Printf("%s%s─%s", s_edge_0, s_edge_1, c)
						}
					} else {
						fmt.Print(p.String())
					}
					i += 1
				}
			}
		},
	}

	list.Flags().BoolVarP(&byExe, "exe", "e", false, "group processes by executable")
	list.Flags().BoolVarP(&lsFds, "fd", "f", false, "list the file descriptors related to each process")
	list.Flags().BoolVarP(&lsSockets, "socket", "s", false, "list the sockets related to each process")
	list.MarkFlagsMutuallyExclusive("exe", "fd", "socket")

	return list
}
