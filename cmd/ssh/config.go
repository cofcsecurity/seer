package ssh

import (
	"fmt"
	"io"
	"path/filepath"
	"seer/pkg/ssh"
	"sort"

	"github.com/spf13/cobra"
)

func Config() *cobra.Command {
	var path, user, addr, host, laddr, lport string
	config := &cobra.Command{Use: "config", Short: "Inspect SSH server configuration"}
	config.PersistentFlags().StringVar(&path, "file", ssh.DefaultConfigPath(), "sshd configuration file")
	list := &cobra.Command{
		Use: "list", Aliases: []string{"ls"}, Short: "List configured SSH rules and their source files",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			rules, err := ssh.ConfigRules(path)
			if err != nil {
				return err
			}
			out, color := cmd.OutOrStdout(), terminalColor(cmd.OutOrStdout())
			for _, r := range rules {
				fmt.Fprint(out, configRuleOutput(r, color))
			}
			return nil
		},
	}
	check := &cobra.Command{
		Use: "check", Short: "Validate SSH configuration and review effective settings",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := ssh.ValidateConfig(path); err != nil {
				return err
			}
			criteria := map[string]string{"user": user, "addr": addr, "host": host, "laddr": laddr, "lport": lport}
			effective, err := ssh.EffectiveConfig(path, criteria)
			if err != nil {
				return err
			}
			out, color := cmd.OutOrStdout(), terminalColor(cmd.OutOrStdout())
			fmt.Fprintln(out, "SSH configuration syntax:", paint("valid", ansiGreen, color))
			if user == "" || addr == "" {
				fmt.Fprintln(out, paint("Note: supply --user and --addr to evaluate connection-specific Match rules.", ansiYellow, color))
			}
			levels := make(map[string]string)
			for _, finding := range ssh.CheckConfig(effective) {
				fmt.Fprint(out, configFindingOutput(finding, color))
				levels[finding.Key] = finding.Level
			}
			keys := make([]string, 0, len(effective))
			for key := range effective {
				keys = append(keys, key)
			}
			sort.Strings(keys)
			for _, key := range keys {
				fmt.Fprint(out, configEffectiveOutput(key, effective[key], levels[key], color))
			}
			printRunningServers(out, path, color)
			return nil
		},
	}
	check.Flags().StringVar(&user, "user", "", "login username for Match rules")
	check.Flags().StringVar(&addr, "addr", "", "client address for Match rules")
	check.Flags().StringVar(&host, "host", "", "client hostname for Match rules")
	check.Flags().StringVar(&laddr, "laddr", "", "server address for Match rules")
	check.Flags().StringVar(&lport, "lport", "", "server port for Match rules")
	config.AddCommand(list, check)
	return config
}

func printRunningServers(out io.Writer, checkedPath string, color bool) {
	servers, err := ssh.RunningServers()
	if err != nil {
		fmt.Fprintln(out, warningOutput(fmt.Sprintf("Runtime SSH inspection unavailable: %v", err), color))
	} else if len(servers) == 0 {
		fmt.Fprintln(out, "No running sshd listener or inetd instance was visible.")
	} else {
		for _, server := range servers {
			fmt.Fprintln(out, paint(fmt.Sprintf("Running sshd PID %d (%s):", server.PID, server.Mode), ansiCyan, color), server.Command)
			for _, listener := range server.Listeners {
				fmt.Fprintln(out, "  "+paint("Listening:", ansiCyan, color), listener)
			}
			if server.ConfigPath != "" && filepath.Clean(server.ConfigPath) != filepath.Clean(checkedPath) {
				fmt.Fprintln(out, paint(fmt.Sprintf("  review: running sshd specifies %s; checked %s", server.ConfigPath, checkedPath), ansiYellow, color))
			}
			for _, override := range server.Overrides {
				fmt.Fprintln(out, paint(fmt.Sprintf("  review: running sshd command-line override %s", override), ansiYellow, color))
			}
		}
	}
	fmt.Fprintln(out, paint("Note: effective settings above read the file on disk; a running daemon may have loaded earlier contents.", ansiYellow, color))
}
