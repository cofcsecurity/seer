package ssh

import (
	"fmt"
	"os"
	"seer/pkg/ssh"
	"seer/pkg/utils"

	"github.com/spf13/cobra"
)

func Keys() *cobra.Command {
	var username, configPath, clientAddr string
	keys := &cobra.Command{Use: "keys", Short: "Inspect and manage authorized SSH keys"}
	keys.PersistentFlags().StringVar(&username, "user", "", "select one account")
	keys.PersistentFlags().StringVar(&configPath, "config", ssh.DefaultConfigPath(), "sshd configuration file")
	keys.PersistentFlags().StringVar(&clientAddr, "addr", "", "client address for Match rules")
	list := &cobra.Command{
		Use: "list", Aliases: []string{"ls"}, Short: "List authorized key fingerprints",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			found, err := ssh.ListKeys(username, configPath, clientAddr)
			if err != nil {
				return err
			}
			for _, key := range found {
				fmt.Print(key.String())
			}
			return nil
		},
	}
	describe := &cobra.Command{
		Use: "describe [fingerprint]", Short: "Describe an authorized key",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			found, err := ssh.ListKeys(username, configPath, clientAddr)
			if err != nil {
				return err
			}
			matches := matchingKeys(found, args[0])
			if len(matches) != 1 {
				return fmt.Errorf("expected one key with that fingerprint, found %d; use --user to narrow it", len(matches))
			}
			fmt.Print(matches[0].Describe())
			return nil
		},
	}
	var yes, allowCurrentAccess bool
	remove := &cobra.Command{
		Use: "remove [fingerprint]", Short: "Remove one authorized key",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			found, err := ssh.ListKeys(username, configPath, clientAddr)
			if err != nil {
				return err
			}
			matches := matchingKeys(found, args[0])
			if len(matches) != 1 {
				return fmt.Errorf("expected one key with that fingerprint, found %d; use --user to narrow it", len(matches))
			}
			fmt.Print(matches[0].Describe())
			if !allowCurrentAccess {
				sessions, err := ssh.ListSessions()
				if err != nil {
					return err
				}
				foundCurrent := false
				for _, session := range sessions {
					foundCurrent = foundCurrent || session.Current
					if session.Current && session.User == "unknown" {
						return fmt.Errorf("current SSH login user could not be identified; use --allow-current-access to override")
					}
					if session.Current && session.User == matches[0].User {
						return fmt.Errorf("key belongs to the current SSH account; use --allow-current-access to override")
					}
				}
				if os.Getenv("SSH_CONNECTION") != "" && !foundCurrent {
					return fmt.Errorf("current SSH session could not be identified; use --allow-current-access to override")
				}
			}
			if !yes && !utils.Confirm() {
				fmt.Println("Canceled.")
				return nil
			}
			backup, err := ssh.RemoveKey(matches[0], configPath, clientAddr)
			if backup != "" {
				fmt.Printf("Backup: %s\n", backup)
			}
			if err != nil {
				return err
			}
			fmt.Println("Removed one authorized key entry.")
			return nil
		},
	}
	remove.Flags().BoolVarP(&yes, "yes", "y", false, "respond to confirmation with yes")
	remove.Flags().BoolVar(&allowCurrentAccess, "allow-current-access", false, "allow removing a key for the current SSH account")
	keys.AddCommand(list, describe, remove)
	return keys
}

func matchingKeys(keys []ssh.AuthorizedKey, fingerprint string) []ssh.AuthorizedKey {
	var result []ssh.AuthorizedKey
	for _, key := range keys {
		if "SHA256:"+key.Fingerprint == fingerprint || key.Fingerprint == fingerprint {
			result = append(result, key)
		}
	}
	return result
}
