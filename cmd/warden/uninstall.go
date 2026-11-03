package warden

import (
	"fmt"
	"os"

	"seer/pkg/warden"

	"github.com/spf13/cobra"
)

func Uninstall() *cobra.Command {
	var name string
	var force, yes bool
	uninstall := &cobra.Command{
		Use: "uninstall", Short: "Remove Warden from this box",
		Long: "Run the install's own uninstall, which removes its units, access account, sshd, sudoers,\n" +
			"cron and firewall registrations, binaries, and local state. Warden refuses while armed\n" +
			"unless --force. Backups already copied to peer boxes are not touched.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.SilenceUsage = true
			if err := requireRoot(); err != nil {
				return err
			}
			in, err := warden.Resolve(name)
			if err != nil {
				return err
			}
			bin := in.Binary
			if in.BinaryMissing {
				bin = in.DataDir + "/.spare"
			}
			wargs := []string{"uninstall"}
			if force {
				wargs = append(wargs, "--force")
			}
			if yes {
				wargs = append(wargs, "--yes")
			}
			out, err := warden.Run(bin, wargs, os.Stdin, cmd.ErrOrStderr())
			cmd.OutOrStdout().Write(out)
			if err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Removed %s. Peer boxes may still hold its backups under /var/lib/<peer>-peers/.\n", in.Name)
			return nil
		},
	}
	uninstall.Flags().StringVar(&name, "name", "", "install name (default: the only install found)")
	uninstall.Flags().BoolVar(&force, "force", false, "uninstall even while armed")
	uninstall.Flags().BoolVarP(&yes, "yes", "y", false, "do not prompt for confirmation")
	return uninstall
}
