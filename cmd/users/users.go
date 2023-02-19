package users

import (
	"github.com/spf13/cobra"
)

func Users() *cobra.Command {
	users := &cobra.Command{
		Use:     "user",
		Aliases: []string{"users"},
		Short:   "Query and manipulate system users",
	}

	users.AddCommand(UsersList())
	users.AddCommand(UsersExpire())
	users.AddCommand(UsersDescribe())

	return users
}
