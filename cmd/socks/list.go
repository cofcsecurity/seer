package socks

import (
	"fmt"
	"seer/pkg/proc"

	"github.com/spf13/cobra"
)

func SocketList() *cobra.Command {
	list := &cobra.Command{
		Use:     "list",
		Aliases: []string{"ls"},
		Short:   "List sockets",
		Run: func(cmd *cobra.Command, args []string) {
			sockets := proc.GetSockets()
			for _, s := range sockets {
				fmt.Print(s.String())
			}
		},
	}

	return list
}
