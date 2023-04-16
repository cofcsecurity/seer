package main

import (
	"seer/cmd/groups"
	"seer/cmd/procs"
	"seer/cmd/socks"
	"seer/cmd/users"

	"github.com/spf13/cobra"
)

func main() {
	root := &cobra.Command{
		Use:   "seer",
		Short: "Seer is a system enumeration and administration tool for linux",
	}

	root.AddCommand(users.Users())
	root.AddCommand(groups.Groups())
	root.AddCommand(procs.Procs())
	root.AddCommand(socks.Socks())

	root.Execute()
}
