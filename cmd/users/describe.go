package users

import (
	"fmt"
	"log/slog"
	"regexp"
	"seer/pkg/users"

	"github.com/spf13/cobra"
)

func UsersDescribe() *cobra.Command {
	var regex, iregex bool

	describe := &cobra.Command{
		Use:   "describe [user | pattern ...]",
		Short: "Describe users",
		Long: `Describe information about a user or users. 
Optionally use regex or inverse regex matching to filter users.`,
		Run: func(cmd *cobra.Command, args []string) {
			users_map, err := users.GetUsers()
			if err != nil {
				slog.Error("Failed to get users", "error", err.Error())
			}
			if len(args) == 0 {
				for _, u := range users_map {
					fmt.Print(u.Describe())
				}
				return
			}
			if regex || iregex {
				if len(args) == 0 {
					fmt.Print("No patterns supplied.\n\n")
					cmd.Help()
					return
				}
				matches := make(map[string]users.User)
				for _, p := range args {
					re, err := regexp.Compile(p)
					if err != nil {
						slog.Warn("Failed to compile pattern. Skipping", "pattern", p)
						continue
					}
					for n, u := range users_map {
						res := re.Match([]byte(n))
						if (res && regex) || (!res && iregex) {
							matches[n] = u
						}
					}
				}
				for _, u := range matches {
					fmt.Print(u.Describe())
				}
			} else {
				for _, u := range args {
					user, exists := users_map[u]
					if exists {
						fmt.Print(user.Describe())
					} else {
						slog.Warn("User does not exist", "user", u)
					}
				}
			}
		},
	}

	describe.Flags().BoolVarP(&regex, "regex", "r", false, "use regex matching")
	describe.Flags().BoolVarP(&iregex, "iregex", "i", false, "use inverse regex matching")
	describe.MarkFlagsMutuallyExclusive("regex", "iregex")

	return describe
}
