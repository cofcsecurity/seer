package ssh

import (
	"fmt"
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
			for _, r := range rules {
				fmt.Printf("%s:%d %s %s\n", r.Path, r.Line, r.Key, r.Value)
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
			fmt.Println("SSH configuration syntax: valid")
			if user == "" || addr == "" {
				fmt.Println("Note: supply --user and --addr to evaluate connection-specific Match rules.")
			}
			for _, finding := range ssh.CheckConfig(effective) {
				fmt.Printf("%s: %s=%s: %s\n", finding.Level, finding.Key, finding.Value, finding.Reason)
			}
			keys := make([]string, 0, len(effective))
			for key := range effective {
				keys = append(keys, key)
			}
			sort.Strings(keys)
			for _, key := range keys {
				fmt.Printf("%s %s\n", key, effective[key])
			}
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
