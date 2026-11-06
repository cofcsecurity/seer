package warden

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"text/tabwriter"

	"seer/pkg/warden"

	"github.com/spf13/cobra"
)

func Profile() *cobra.Command {
	var name string
	var roleNames []string
	var dryRun, printJSON, yes bool
	profile := &cobra.Command{
		Use:   "profile [--role auto|" + strings.Join(warden.RoleNames(), ",") + "]",
		Short: "Add the important files for this box's role to Warden's watch list",
		Long: "Warden watches accounts, sshd, sudoers, cron, units and the main config of common services\n" +
			"out of the box. This adds what those defaults miss, from role templates, to the install's\n" +
			"host profile (/var/lib/<name>/profile.json). Only files that exist on this box are added,\n" +
			"and paths already in the profile are kept as they are.\n\n" +
			"Roles (base always applies; auto picks every role with files present):\n" + roleHelp() + "\n\n" +
			"Firewall rules and share or app folders are confirm-first: Warden flags drift there instead\n" +
			"of undoing it, because the team and the scorer change them on purpose. Live database files\n" +
			"are never added; copying them back corrupts the database.\n\n" +
			"After writing, Seer snapshots the new paths and runs Warden's 'profile validate'.",
		Example: "  seer warden profile --dry-run\n" +
			"  seer warden profile --role db --yes\n" +
			"  seer warden profile --print > profile.json",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.SilenceUsage = true
			if printJSON {
				proposed, _, err := warden.ProposeProfile(roleNames)
				if err != nil {
					return err
				}
				entries := make([]warden.ProfileEntry, len(proposed))
				for i, e := range proposed {
					entries[i] = e.ProfileEntry
				}
				enc := json.NewEncoder(cmd.OutOrStdout())
				enc.SetIndent("", "  ")
				return enc.Encode(map[string]any{"paths": entries})
			}
			if err := requireRoot(); err != nil {
				return err
			}
			in, err := warden.Resolve(name)
			if err != nil {
				return err
			}
			return applyProfile(cmd, in, roleNames, yes, dryRun)
		},
	}
	f := profile.Flags()
	f.StringSliceVar(&roleNames, "role", nil, "roles to apply, comma-separated (default: auto)")
	f.StringVar(&name, "name", "", "install name (default: the only install found)")
	f.BoolVar(&dryRun, "dry-run", false, "show what would be added without changing anything")
	f.BoolVar(&printJSON, "print", false, "print the proposed profile as JSON; needs no install")
	f.BoolVarP(&yes, "yes", "y", false, "do not ask before writing")
	profile.RegisterFlagCompletionFunc("role", func(*cobra.Command, []string, string) ([]string, cobra.ShellCompDirective) {
		return append([]string{"auto"}, warden.RoleNames()...), cobra.ShellCompDirectiveNoFileComp
	})
	return profile
}

func roleHelp() string {
	var b strings.Builder
	for _, r := range warden.Roles() {
		fmt.Fprintf(&b, "  %-10s %s\n", r.Name, r.Summary)
	}
	return strings.TrimRight(b.String(), "\n")
}

// applyProfile proposes role entries, shows them, and on confirmation writes
// them to the install's profile, snapshots them, and validates the result.
func applyProfile(cmd *cobra.Command, in warden.Install, roleNames []string, yes, dryRun bool) error {
	out, log := cmd.OutOrStdout(), cmd.ErrOrStderr()
	color := terminalColor(out)
	proposed, applied, err := warden.ProposeProfile(roleNames)
	if err != nil {
		return err
	}
	file := in.ProfilePath()
	added, content, err := warden.MergeProfile(file, proposed)
	if err != nil {
		return err
	}
	fmt.Fprintf(log, "==> Roles for this box: %s\n", strings.Join(applied, ", "))
	if len(added) == 0 {
		fmt.Fprintf(out, "Nothing to add: %s already lists every template file found here.\n", file)
		return nil
	}

	fmt.Fprintf(out, "Add to %s:\n", file)
	tw := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
	for _, e := range added {
		class := e.Class
		if class == warden.ConfirmFirst {
			class = paint(class, ansiYellow, color)
		}
		fmt.Fprintf(tw, "  %s\t%s\t%s %s\t%s\n", e.Role, e.Path, e.Tier, class, e.Why)
	}
	tw.Flush()
	if dryRun {
		fmt.Fprintln(out, "Dry run: nothing written.")
		return nil
	}
	if !yes {
		ok, err := confirm(cmd.InOrStdin(), out, fmt.Sprintf("Add these %d paths?", len(added)))
		if err != nil {
			return err
		}
		if !ok {
			fmt.Fprintln(out, "Nothing written.")
			return nil
		}
	}

	backup, err := warden.WriteProfile(file, content)
	if err != nil {
		return err
	}
	if backup != "" {
		fmt.Fprintf(log, "==> Previous profile kept as %s\n", backup)
	}
	fmt.Fprintf(out, "%s %d paths to %s\n", paint("Added", ansiGreen, color), len(added), file)

	tiers := map[string]bool{}
	for _, e := range added {
		tiers[e.Tier] = true
	}
	for _, t := range []string{warden.TierConfig, warden.TierData} {
		if !tiers[t] {
			continue
		}
		fmt.Fprintf(log, "==> Snapshotting the %s tier\n", t)
		if err := warden.RunTo(in.Binary, []string{"snapshot", "--tier", t}, nil, io.Discard, log); err != nil {
			return fmt.Errorf("the profile is written, but the %s snapshot failed: %w", t, err)
		}
	}

	issues, err := warden.ValidationIssues(in)
	if err != nil {
		return fmt.Errorf("the profile is written, but Warden could not validate it: %w", err)
	}
	if len(issues) == 0 {
		fmt.Fprintln(out, "Warden validated the profile: no issues.")
		return nil
	}
	fmt.Fprintln(out, "Warden's profile check:")
	for _, line := range groupIssues(issues) {
		fmt.Fprintf(out, "  %s %s\n", paint("•", ansiYellow, color), line)
	}
	return nil
}

// groupIssues folds "kind: value" issues of the same kind into one line, so
// a box with many unrecognized processes reads as one finding, not twenty.
func groupIssues(issues []string) []string {
	var order []string
	values := map[string][]string{}
	for _, i := range issues {
		kind, value, ok := strings.Cut(i, ": ")
		if !ok {
			kind, value = i, ""
		}
		if _, seen := values[kind]; !seen {
			order = append(order, kind)
		}
		if value != "" {
			values[kind] = append(values[kind], value)
		} else if values[kind] == nil {
			values[kind] = []string{}
		}
	}
	lines := make([]string, 0, len(order))
	for _, kind := range order {
		switch v := values[kind]; len(v) {
		case 0:
			lines = append(lines, kind)
		case 1:
			lines = append(lines, kind+": "+v[0])
		default:
			lines = append(lines, fmt.Sprintf("%s (%d): %s", kind, len(v), strings.Join(v, ", ")))
		}
	}
	return lines
}

func confirm(in io.Reader, out io.Writer, question string) (bool, error) {
	if f, ok := in.(*os.File); ok {
		if fi, err := f.Stat(); err != nil || fi.Mode()&os.ModeCharDevice == 0 {
			return false, fmt.Errorf("%s needs an answer; run with --yes to write without asking, or --dry-run to only look", question)
		}
	}
	fmt.Fprintf(out, "%s [y/N] ", question)
	line, err := bufio.NewReader(in).ReadString('\n')
	if err != nil && line == "" {
		return false, err
	}
	answer := strings.ToLower(strings.TrimSpace(line))
	return answer == "y" || answer == "yes", nil
}
