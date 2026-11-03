package warden

import (
	"errors"
	"fmt"
	"time"

	"seer/pkg/warden"

	"github.com/spf13/cobra"
)

func Status() *cobra.Command {
	var name string
	var jsonOut bool
	status := &cobra.Command{
		Use: "status", Short: "Report the health of the Warden install on this box",
		Long: "Run the install's 'status --json' and check it against Warden's health rules: installed,\n" +
			"armed, snapshot generations, store integrity, component freshness, peers, and pending\n" +
			"drift. Exits with an error when something is broken.",
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
			if in.BinaryMissing {
				return fmt.Errorf("%s is missing; the sentinel restores it from .spare within ~2 minutes. If it stays missing, re-run seer warden install", in.Binary)
			}
			raw, err := warden.Run(in.Binary, []string{"status", "--json"}, nil, cmd.ErrOrStderr())
			if err != nil {
				return err
			}
			if jsonOut {
				_, err := cmd.OutOrStdout().Write(raw)
				return err
			}
			s, err := warden.ParseStatus(raw)
			if err != nil {
				return err
			}
			if s.Name == "" {
				s.Name = in.Name
			}
			findings := warden.Health(s, time.Now())

			out := cmd.OutOrStdout()
			color := terminalColor(out)
			fmt.Fprintf(out, "┌ %s on %s\n", paint(s.Name, ansiCyan, color), s.Host)
			labels := map[warden.Severity]string{
				warden.OK:   paint("ok  ", ansiGreen, color),
				warden.Warn: paint("warn", ansiYellow, color),
				warden.Bad:  paint("FAIL", ansiRed, color),
			}
			for i, f := range findings {
				branch, rail := "├", "│"
				if i == len(findings)-1 {
					branch, rail = "└", " "
				}
				fmt.Fprintf(out, "%s %s %s\n", branch, labels[f.Severity], f.Message)
				if f.Action != "" {
					fmt.Fprintf(out, "%s      → %s\n", rail, f.Action)
				}
			}
			if warden.Worst(findings) == warden.Bad {
				return errors.New("Warden is not healthy")
			}
			return nil
		},
	}
	status.Flags().StringVar(&name, "name", "", "install name (default: the only install found)")
	status.Flags().BoolVar(&jsonOut, "json", false, "print Warden's status JSON unchanged")
	return status
}
