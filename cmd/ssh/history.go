package ssh

import (
	"fmt"
	"seer/pkg/ssh"

	"github.com/spf13/cobra"
)

func History() *cobra.Command {
	var failed bool
	var limit int
	history := &cobra.Command{
		Use: "history", Short: "Show available login records and SSH daemon events",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			output, err := ssh.LoginHistory(failed, limit)
			if err != nil {
				return err
			}
			fmt.Fprint(cmd.OutOrStdout(), historyOutput(output, failed, terminalColor(cmd.OutOrStdout())))
			return nil
		},
	}
	history.Flags().BoolVar(&failed, "failed", false, "show failed SSH authentication events and bad system login records")
	history.Flags().IntVarP(&limit, "limit", "n", 20, "maximum records from each source (1-500)")
	return history
}
