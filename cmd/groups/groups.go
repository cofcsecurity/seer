package groups

import (
	"github.com/spf13/cobra"
)

func Groups() *cobra.Command {
	groups := &cobra.Command{
		Use:     "group",
		Aliases: []string{"groups"},
		Short:   "Query and manipulate system groups",
	}

	groups.AddCommand(GroupList())

	return groups
}
