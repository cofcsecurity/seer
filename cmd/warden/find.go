package warden

import (
	"fmt"

	"seer/pkg/warden"

	"github.com/spf13/cobra"
)

func Find() *cobra.Command {
	return &cobra.Command{
		Use: "find", Short: "List Warden installs on this box",
		Long: "Warden installs under a random name. Find looks for /var/lib/<name>/config.json beside a\n" +
			".spare binary copy and reports each install's binary, data, and config paths.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			found, err := warden.Find()
			if err != nil {
				return err
			}
			out := cmd.OutOrStdout()
			if len(found) == 0 {
				fmt.Fprintln(out, "No Warden install found.")
				return nil
			}
			color := terminalColor(out)
			for _, in := range found {
				line := fmt.Sprintf("[%s] %s data:%s", paint(in.Name, ansiCyan, color), in.Binary, in.DataDir)
				if in.BinaryMissing {
					line += " " + paint("[BINARY MISSING]", ansiYellow, color)
				}
				fmt.Fprintln(out, line)
			}
			return nil
		},
	}
}
