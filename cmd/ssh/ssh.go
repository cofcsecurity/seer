package ssh

import "github.com/spf13/cobra"

func SSH() *cobra.Command {
	ssh := &cobra.Command{
		Use:   "ssh",
		Short: "Inspect and manage SSH connections, configuration, and keys",
	}
	ssh.AddCommand(SessionList())
	ssh.AddCommand(SessionDescribe())
	ssh.AddCommand(SessionEnd())
	ssh.AddCommand(History())
	ssh.AddCommand(Config())
	ssh.AddCommand(Keys())
	return ssh
}
