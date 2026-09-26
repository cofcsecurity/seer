package ssh

import (
	"fmt"
	"seer/pkg/ssh"

	"github.com/spf13/cobra"
)

func SessionList() *cobra.Command {
	return &cobra.Command{
		Use: "list", Aliases: []string{"ls"},
		Short: "List live SSH connections",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			sessions, err := ssh.ListSessions()
			if err != nil {
				return err
			}
			color := terminalColor(cmd.OutOrStdout())
			for _, s := range sessions {
				fmt.Fprint(cmd.OutOrStdout(), sessionOutput(s, false, color))
			}
			return nil
		},
	}
}
