package ssh

import (
	"fmt"
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
	var describePath string
	var describeLine int
	describe := &cobra.Command{
		Use: "describe [fingerprint]", Short: "Describe an authorized key",
		Args:              cobra.ExactArgs(1),
		ValidArgsFunction: completeKeyFingerprints(&username, &configPath, &clientAddr),
		RunE: func(cmd *cobra.Command, args []string) error {
			found, err := ssh.ListKeys(username, configPath, clientAddr)
			if err != nil {
				return err
			}
			matches := matchingKeys(found, args[0], describePath, describeLine)
			if len(matches) != 1 {
				return fmt.Errorf("expected one key with that fingerprint, found %d; use --user, --path, and --line to narrow it", len(matches))
			}
			fmt.Print(matches[0].Describe())
			return nil
		},
	}
	describe.Flags().StringVar(&describePath, "path", "", "authorized_keys file containing the key")
	describe.Flags().IntVar(&describeLine, "line", 0, "line number of the key")
	var yes, allowCurrentAccess bool
	var removePath string
	var removeLine int
	remove := &cobra.Command{
		Use: "remove [fingerprint]", Short: "Remove one authorized key",
		Args:              cobra.ExactArgs(1),
		ValidArgsFunction: completeKeyFingerprints(&username, &configPath, &clientAddr),
		RunE: func(cmd *cobra.Command, args []string) error {
			found, err := ssh.ListKeys(username, configPath, clientAddr)
			if err != nil {
				return err
			}
			matches := matchingKeys(found, args[0], removePath, removeLine)
			if len(matches) != 1 {
				return fmt.Errorf("expected one key with that fingerprint, found %d; use --user, --path, and --line to narrow it", len(matches))
			}
			fmt.Print(matches[0].Describe())
			if !allowCurrentAccess {
				sessions, status, err := ssh.ListSessionsWithStatus()
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
				if (ssh.SSHContextPresent() || status.Incomplete) && !foundCurrent {
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
	remove.Flags().StringVar(&removePath, "path", "", "authorized_keys file containing the key")
	remove.Flags().IntVar(&removeLine, "line", 0, "line number of the key")
	sources := &cobra.Command{
		Use: "sources", Short: "Show SSH key, certificate, and host-key sources for one account",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			report, err := ssh.AuthenticationSources(username, configPath, clientAddr)
			if err != nil {
				return err
			}
			fmt.Fprint(cmd.OutOrStdout(), report.String())
			return nil
		},
	}
	keys.AddCommand(list, describe, remove, sources)
	return keys
}

func matchingKeys(keys []ssh.AuthorizedKey, fingerprint, path string, line int) []ssh.AuthorizedKey {
	var result []ssh.AuthorizedKey
	for _, key := range keys {
		if ("SHA256:"+key.Fingerprint == fingerprint || key.Fingerprint == fingerprint) &&
			(path == "" || key.Path == path) && (line == 0 || key.Line == line) {
			result = append(result, key)
		}
	}
	return result
}
