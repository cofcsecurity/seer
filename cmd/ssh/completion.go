package ssh

import (
	"fmt"
	"sort"
	"strings"

	"github.com/spf13/cobra"
	"seer/pkg/ssh"
)

func completeSessionIDs(inboundOnly bool) func(*cobra.Command, []string, string) ([]string, cobra.ShellCompDirective) {
	return func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		if len(args) != 0 {
			return nil, cobra.ShellCompDirectiveNoFileComp
		}
		sessions, err := ssh.ListSessions()
		if err != nil {
			return nil, cobra.ShellCompDirectiveNoFileComp
		}
		return sessionIDCompletions(sessions, inboundOnly, toComplete), cobra.ShellCompDirectiveNoFileComp
	}
}

func completeKeyFingerprints(username, configPath, clientAddr *string) func(*cobra.Command, []string, string) ([]string, cobra.ShellCompDirective) {
	return func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		if len(args) != 0 {
			return nil, cobra.ShellCompDirectiveNoFileComp
		}
		keys, err := ssh.ListKeys(*username, *configPath, *clientAddr)
		if err != nil {
			return nil, cobra.ShellCompDirectiveNoFileComp
		}
		return keyFingerprintCompletions(keys, toComplete), cobra.ShellCompDirectiveNoFileComp
	}
}

func sessionIDCompletions(sessions []ssh.Session, inboundOnly bool, prefix string) []string {
	var matches []string
	for _, session := range sessions {
		if (inboundOnly && session.Direction != "inbound") || !strings.HasPrefix(session.ID, prefix) {
			continue
		}
		description := fmt.Sprintf("%s %s", session.Direction, session.Remote)
		if session.Current {
			description += " CURRENT SESSION"
		}
		matches = append(matches, session.ID+"\t"+description)
	}
	return matches
}

func keyFingerprintCompletions(keys []ssh.AuthorizedKey, prefix string) []string {
	seen := make(map[string]bool)
	var matches []string
	for _, key := range keys {
		fingerprint := "SHA256:" + key.Fingerprint
		if seen[fingerprint] || !strings.HasPrefix(fingerprint, prefix) {
			continue
		}
		seen[fingerprint] = true
		matches = append(matches, fmt.Sprintf("%s\t%s %s:%d", fingerprint, key.User, key.Path, key.Line))
	}
	sort.Strings(matches)
	return matches
}
