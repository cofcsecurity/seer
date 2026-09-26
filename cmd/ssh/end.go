package ssh

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"seer/pkg/ssh"
	"seer/pkg/utils"
	"strings"

	"github.com/spf13/cobra"
)

func SessionEnd() *cobra.Command {
	var yes bool
	end := &cobra.Command{
		Use: "end [session-id]", Short: "End one inbound SSH connection",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			sessions, err := ssh.ListSessions()
			if err != nil {
				return err
			}
			var target *ssh.Session
			foundCurrent := false
			for _, s := range sessions {
				foundCurrent = foundCurrent || s.Current
				if s.ID == args[0] {
					copy := s
					target = &copy
				}
			}
			if target == nil {
				return fmt.Errorf("SSH session %q is no longer visible", args[0])
			}
			fmt.Fprint(cmd.OutOrStdout(), sessionOutput(*target, true, terminalColor(cmd.OutOrStdout())))
			confirmed, currentRisk := confirmSessionEnd(cmd, *target, foundCurrent, yes)
			if !confirmed {
				fmt.Fprintln(cmd.OutOrStdout(), "Canceled.")
				return nil
			}
			if err := ssh.EndSession(args[0], currentRisk); err != nil {
				return err
			}
			fmt.Fprintln(cmd.OutOrStdout(), "SIGTERM sent to SSH connection process.")
			return nil
		},
	}
	end.Flags().BoolVarP(&yes, "yes", "y", false, "skip confirmation for connections other than the current one")
	return end
}

func confirmSessionEnd(cmd *cobra.Command, target ssh.Session, foundCurrent, yes bool) (bool, bool) {
	currentRisk := target.Current || (os.Getenv("SSH_CONNECTION") != "" && !foundCurrent)
	if currentRisk {
		var warning string
		if target.Current {
			warning = "WARNING: This is your current SSH connection. Ending it will disconnect this terminal."
		} else {
			warning = "WARNING: Your current SSH connection could not be identified. This may be your own connection."
		}
		fmt.Fprintln(cmd.OutOrStdout(), warningOutput(warning, terminalColor(cmd.OutOrStdout())))
		fmt.Fprintln(cmd.OutOrStdout(), "--yes does not skip confirmation for a possible current connection.")
		return confirmCurrentEnd(cmd.InOrStdin(), cmd.OutOrStdout()), true
	}
	return yes || utils.Confirm(), false
}

func confirmCurrentEnd(in io.Reader, out io.Writer) bool {
	reader := bufio.NewReader(in)
	fmt.Fprint(out, "End this possible current connection? (yes/no): ")
	answer, err := reader.ReadString('\n')
	if err != nil || strings.TrimSpace(answer) != "yes" {
		return false
	}
	fmt.Fprint(out, "Confirm again to end this SSH connection? (yes/no): ")
	answer, err = reader.ReadString('\n')
	return err == nil && strings.TrimSpace(answer) == "yes"
}
