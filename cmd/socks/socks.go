package socks

import (
	"github.com/spf13/cobra"
)

func Socks() *cobra.Command {
	socks := &cobra.Command{
		Use:     "socks",
		Aliases: []string{"sock"},
		Short:   "Query sockets",
	}

	socks.AddCommand(SocketList())
	socks.AddCommand(SocketDescribe())

	return socks
}
