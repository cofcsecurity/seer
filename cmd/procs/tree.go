package procs

import (
	"fmt"
	"seer/pkg/proc"
	"strconv"

	"github.com/spf13/cobra"
)

func printTree(root_pid int, procs map[int]proc.Process) {
	root_i := -1
	for i, p := range procs {
		if p.Pid == root_pid {
			root_i = i
			break
		}
	}
	if root_i == -1 && root_pid != 0 {
		fmt.Printf("The process '%d' does not exist.\n", root_pid)
		return
	}

	// Map of pid -> number of children visited
	// If a pid is in the map, it has itself been visited
	visits := make(map[int]int)

	var dfs func(root int, subtree_root int, current proc.Process, procs map[int]proc.Process)
	dfs = func(root int, subtree_root int, current proc.Process, procs map[int]proc.Process) {
		// Mark vertex as visted
		visits[current.Pid] = 0
		if current.Ppid != 0 {
			if _, exists := visits[current.Ppid]; exists {
				visits[current.Ppid] += 1
			}
		}
		// Add edges
		if !(current.Pid == subtree_root) {
			parents := current.GetParents(procs)
			edges := make([]string, 0)
			for i := 0; i < len(parents); i++ {
				if parents[i].Pid == current.Ppid {
					// Edge to connect to this vertex's parent
					if visits[parents[i].Pid] != len(parents[i].Children) {
						// If there are other unvisited children at this level
						edges = append(edges, "├")
					} else {
						// If this is the last vertex at this level of the subtree
						edges = append(edges, "└")
					}
				} else if visits[parents[i].Pid] != len(parents[i].Children) {
					// For non parent ancestors with unvisted children
					edges = append(edges, "│")
				} else {
					// For non parent ancestors with no unvisited children
					edges = append(edges, " ")
				}
				if parents[i].Pid == root {
					// If we are printing a subset of all processes a proceess will
					// have more parents than we should actually print edges for
					break
				}
			}
			// Edges are reversed
			for i := len(edges) - 1; i >= 0; i-- {
				fmt.Print(edges[i])
			}
		}
		// Add an edge to connect to any children of this process if needed
		if len(current.Children) > 0 {
			fmt.Print("┬")
		} else {
			fmt.Print("─")
		}
		fmt.Print(current.String())

		for _, c := range current.Children {
			dfs(root, current.Pid, procs[c], procs)
		}
	}

	if root_pid != 0 {
		dfs(root_pid, root_pid, procs[root_i], procs)
	} else {
		for _, p := range procs {
			if p.Ppid == root_pid {
				dfs(p.Pid, p.Pid, p, procs)
			}
		}
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
			procs := proc.GetProcesses()
			printTree(root, procs)
		},
	}

	return tree
}
