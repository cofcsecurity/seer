package users

import (
	"fmt"
	"log/slog"
	"regexp"

	"seer/pkg/users"
	"seer/pkg/utils"

	"github.com/spf13/cobra"
)

func UsersExpire() *cobra.Command {
	var unexpire, yes bool
	var regex, iregex bool

	expire := &cobra.Command{
		Use:   "expire [user | pattern ...]",
		Short: "Expire or unexpire users on the system",
		Args:  cobra.MinimumNArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			users_map, err := users.GetUsers()
			if err != nil {
				slog.Error("Failed to get users", "error", err.Error())
				return
			}
			targets := make([]string, 0)
			if regex || iregex {
				matches := make(map[string]users.User)
				for _, p := range args {
					re, err := regexp.Compile(p)
					if err != nil {
						slog.Warn("Pattern failed to complie. Skipping", "pattern", p)
						continue
					}
					for n, u := range users_map {
						res := re.Match([]byte(n))
						if (res && regex) || (!res && iregex) {
							matches[n] = u
						}
					}
				}
				for n := range matches {
					targets = append(targets, n)
				}
			} else {
				targets = args
			}
			if len(targets) == 0 {
				fmt.Printf("No matching users found.\n")
				return
			}
			fmt.Printf("The following %d user(s) will be modified:\n", len(targets))
			for _, u := range targets {
				fmt.Printf("  %s\n", u)
			}
			if !yes && !utils.Confirm() {
				fmt.Printf("Canceled.\n")
				return
			}
			modified := 0
			for _, u := range targets {
				user, exists := users_map[u]
				if exists {
					if unexpire {
						err = user.UnExpire()
					} else {
						err = user.Expire()
					}
					if err != nil {
						slog.Error("Failed to (un)expire user", "user", u, "error", err.Error())
					} else {
						modified += 1
					}
				} else {
					slog.Warn("User does not exist", "user", u)
				}
			}
			fmt.Printf("Modified %d user(s).\n", modified)
		},
	}

	expire.Flags().BoolVarP(&unexpire, "unexpire", "u", false, "unexpire users")
	expire.Flags().BoolVarP(&yes, "yes", "y", false, "respond to prompts with yes")
	expire.Flags().BoolVarP(&regex, "regex", "r", false, "use regex matching")
	expire.Flags().BoolVarP(&iregex, "iregex", "i", false, "use inverse regex matching")
	expire.MarkFlagsMutuallyExclusive("regex", "iregex")

	return expire
}
