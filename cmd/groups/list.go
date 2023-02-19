package groups

import (
	"fmt"
	"seer/pkg/users"
	"sort"

	"github.com/spf13/cobra"
)

func GroupList() *cobra.Command {
	list := &cobra.Command{
		Use:     "list",
		Aliases: []string{"ls"},
		Short:   "List the groups on the system",
		Run: func(cmd *cobra.Command, args []string) {
			groups_map, err := users.GetGroups()
			if err != nil {
				fmt.Printf("%s\n", err)
			}
			groups := make([]users.Group, 0)
			for _, g := range groups_map {
				groups = append(groups, g)
			}
			sort.Slice(groups, func(i, j int) bool { return groups[i].Id < groups[j].Id })
			for _, g := range groups {
				fmt.Print(g.Describe())
			}
		},
	}

	return list
}
