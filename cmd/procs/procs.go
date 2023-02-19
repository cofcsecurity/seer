package procs

import (
	"github.com/spf13/cobra"
)

func Procs() *cobra.Command {
	procs := &cobra.Command{
		Use:     "proc",
		Aliases: []string{"procs"},
		Short:   "Query information about processes running on the system",
	}

	procs.AddCommand(ProcsList())

	return procs
}
