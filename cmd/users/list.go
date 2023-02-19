package users

import (
	"fmt"
	"seer/pkg/users"
	"sort"

	"github.com/spf13/cobra"
)

func UsersList() *cobra.Command {
	list := &cobra.Command{
		Use:     "list",
		Aliases: []string{"ls"},
		Short:   "List the users on the system",
		Run: func(cmd *cobra.Command, args []string) {
			users_map, err := users.GetUsers()
			if err != nil {
				fmt.Printf("%s\n", err)
			}
			users := make([]users.User, 0)
			for _, u := range users_map {
				users = append(users, u)
			}

			sort.Slice(users, func(i, j int) bool { return users[i].Uid < users[j].Uid })
			for _, u := range users {
				fmt.Print(u.String())
			}
		},
	}

	return list
}
