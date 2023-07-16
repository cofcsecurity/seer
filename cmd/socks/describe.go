package socks

import (
	"fmt"
	"seer/pkg/proc"

	"github.com/spf13/cobra"
)

func SocketDescribe() *cobra.Command {
	describe := &cobra.Command{
		Use:   "describe",
		Short: "Describe sockets",
		Run: func(cmd *cobra.Command, args []string) {
			sockets := proc.GetSockets()
			for _, s := range sockets {
				fmt.Print(s.Describe())
			}
		},
	}

	return describe
}
