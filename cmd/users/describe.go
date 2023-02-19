package users

import (
	"fmt"
	"log"
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
		Args: cobra.MinimumNArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			users_map, err := users.GetUsers()
			if err != nil {
				log.Printf("%s\n", err)
			}
			if regex || iregex {
				matches := make(map[string]users.User)
				for _, p := range args {
					re, err := regexp.Compile(p)
					if err != nil {
						log.Printf("Warning: the pattern '%s' failed to complie. Skipping.\n", p)
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
						log.Printf("The user '%s' does not exist", u)
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
