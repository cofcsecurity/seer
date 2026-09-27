package ssh

import (
	"fmt"
	"seer/pkg/ssh"

	"github.com/spf13/cobra"
)

func SessionDescribe() *cobra.Command {
	return &cobra.Command{
		Use: "describe [session-id]", Short: "Describe a live SSH connection",
		Args:              cobra.ExactArgs(1),
		ValidArgsFunction: completeSessionIDs(false),
		RunE: func(cmd *cobra.Command, args []string) error {
			sessions, err := ssh.ListSessions()
			if err != nil {
				return err
			}
			for _, s := range sessions {
				if s.ID == args[0] {
					fmt.Fprint(cmd.OutOrStdout(), sessionOutput(s, true, terminalColor(cmd.OutOrStdout())))
					return nil
				}
			}
			return fmt.Errorf("SSH session %q is no longer visible", args[0])
		},
	}
}
