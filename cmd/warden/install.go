package warden

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"seer/pkg/warden"

	"github.com/spf13/cobra"
)

func Install() *cobra.Command {
	var opts warden.InstallOptions
	var variant, release, ref, source, binary, pin string
	var jsonOut bool
	install := &cobra.Command{
		Use:   "install --key <team-pub | @file | https://url>",
		Short: "Obtain and install Warden or Quiet Warden on this box",
		Long: "Obtain Warden and run its install on this box. By default Seer clones the repository and\n" +
			"builds it with Go; --release downloads a GitHub release and verifies it against the\n" +
			"release's SHA256SUMS; --source builds an existing clone; --binary uses a binary already here.\n\n" +
			"Use --variant quiet-warden for Quiet Warden. Both repositories are private: Seer checks\n" +
			"access before downloading and explains what to fix when the token is missing or lacks access.\n\n" +
			"Installing again updates the existing install in place. Record the printed name: it is the\n" +
			"binary, the access account, the unit prefix, and the data directory.",
		Example: "  sudo --preserve-env=GITHUB_TOKEN seer warden install --key @team.pub --from 10.0.0.0/24 --yes\n" +
			"  seer warden install --variant quiet-warden --release latest --key @team.pub --safe-accounts admin\n" +
			"  seer warden install --binary ./warden-linux-amd64 --sha256 <hash> --key @team.pub",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.SilenceUsage = true
			if err := requireRoot(); err != nil {
				return err
			}
			v, err := warden.LookupVariant(variant)
			if err != nil {
				return err
			}
			if n := countSet(release, source, binary); n > 1 {
				return errors.New("use only one of --release, --source, and --binary")
			}
			if ref != "" && (release != "" || source != "" || binary != "") {
				return errors.New("--ref applies only when Seer clones the repository")
			}
			if pin != "" && release == "" && binary == "" {
				return errors.New("--sha256 applies only to --release or --binary")
			}
			if !warden.ValidMode(opts.Ban) || !warden.ValidMode(opts.Lock) {
				return errors.New("--ban and --lock take off, flag, or act")
			}
			if opts.Name != "" && !warden.ValidName(opts.Name) {
				return fmt.Errorf("invalid --name %q", opts.Name)
			}
			if opts.Key == "" {
				if found, _ := warden.Find(); len(found) == 0 {
					return errors.New("--key is required on first install (team public key, @file, or https URL)")
				}
			} else {
				// Check the key before any clone or download, so a bad key fails fast.
				key, err := warden.ResolveKey(cmd.Context(), opts.Key)
				if err != nil {
					return err
				}
				opts.Key = key
			}

			log := cmd.ErrOrStderr()
			work, err := workDir()
			if err != nil {
				return err
			}
			defer os.RemoveAll(work)

			bin, err := obtain(cmd.Context(), v, release, ref, source, binary, pin, work, log)
			if err != nil {
				return err
			}

			fmt.Fprintf(log, "==> Installing %s\n", v.Name)
			out, err := warden.Run(bin, opts.Args(), cmd.InOrStdin(), log)
			if err != nil {
				return err
			}
			result, err := warden.ParseInstall(out)
			if err != nil {
				return err
			}
			if jsonOut {
				_, err := cmd.OutOrStdout().Write(out)
				return err
			}
			printInstall(cmd.OutOrStdout(), result, v)
			return nil
		},
	}
	f := install.Flags()
	f.StringVar(&variant, "variant", "warden", "which build to install: warden or quiet-warden")
	f.StringVar(&release, "release", "", "download this GitHub release tag (or latest) instead of building")
	f.StringVar(&ref, "ref", "", "branch or tag to clone (default: the repository's default branch)")
	f.StringVar(&source, "source", "", "build from this existing clone instead of cloning")
	f.StringVar(&binary, "binary", "", "install this Warden binary instead of downloading one")
	f.StringVar(&pin, "sha256", "", "require this SHA-256 for the release or --binary")
	f.StringVar(&opts.Key, "key", "", "team SSH public key, @file, or https:// URL (required on first install)")
	f.StringVar(&opts.From, "from", "", `team network: IPs/CIDRs, comma-separated, or "any" (default: the SSH client's /24)`)
	f.StringVar(&opts.Name, "name", "", "install name (default: Warden picks one that blends in)")
	f.StringVar(&opts.Ban, "ban", "", "IP ban response: off, flag, or act (default flag)")
	f.StringVar(&opts.Lock, "lock", "", "account lock response: off, flag, or act (default flag)")
	f.StringSliceVar(&opts.SafeAccounts, "safe-accounts", nil, "accounts Warden never locks (team and scoring accounts)")
	f.BoolVarP(&opts.Yes, "yes", "y", false, "do not prompt; accept defaults")
	f.BoolVar(&jsonOut, "json", false, "print Warden's install result as JSON")
	_ = install.RegisterFlagCompletionFunc("variant", func(*cobra.Command, []string, string) ([]string, cobra.ShellCompDirective) {
		return warden.VariantNames(), cobra.ShellCompDirectiveNoFileComp
	})
	return install
}

// workDir makes a private scratch directory for the download or build. It
// prefers root's home over /tmp, which hardened boxes often mount noexec.
func workDir() (string, error) {
	if home, err := os.UserHomeDir(); err == nil {
		if dir, err := os.MkdirTemp(home, ".seer-warden-"); err == nil {
			return dir, nil
		}
	}
	return os.MkdirTemp("", "seer-warden-")
}

func countSet(values ...string) int {
	n := 0
	for _, v := range values {
		if v != "" {
			n++
		}
	}
	return n
}

// obtain returns the path of a Warden binary to run install with.
func obtain(ctx context.Context, v warden.Variant, release, ref, source, binary, pin, work string, log io.Writer) (string, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if binary != "" {
		return checkBinary(binary, pin)
	}
	bin := filepath.Join(work, "warden")
	if source != "" {
		fmt.Fprintf(log, "==> Building %s from %s\n", v.Name, source)
		return bin, warden.Build(ctx, source, bin, work, log)
	}

	token, tokenSource := warden.Token()
	fmt.Fprintf(log, "==> Checking access to github.com/%s\n", v.Repo)
	if err := warden.CheckAccess(ctx, v.Repo, token, tokenSource); err != nil {
		return "", err
	}

	if release != "" {
		arch := runtime.GOARCH
		if arch != "amd64" && arch != "arm64" {
			return "", fmt.Errorf("no Warden release for %s; use --source or --binary", arch)
		}
		fmt.Fprintf(log, "==> Downloading %s release %s for linux/%s\n", v.Name, release, arch)
		tag, sum, err := warden.DownloadRelease(ctx, v.Repo, release, arch, token, tokenSource, bin, pin)
		if err != nil {
			return "", err
		}
		fmt.Fprintf(log, "==> Verified %s %s (sha256 %s)\n", v.Name, tag, sum)
		return bin, nil
	}

	src := filepath.Join(work, "src")
	fmt.Fprintf(log, "==> Cloning github.com/%s\n", v.Repo)
	if err := warden.Clone(ctx, v.Repo, ref, token, tokenSource, src); err != nil {
		return "", err
	}
	fmt.Fprintf(log, "==> Building %s\n", v.Name)
	return bin, warden.Build(ctx, src, bin, work, log)
}

func checkBinary(binary, pin string) (string, error) {
	f, err := os.Open(binary)
	if err != nil {
		return "", err
	}
	defer f.Close()
	if info, err := f.Stat(); err != nil || info.IsDir() {
		return "", fmt.Errorf("%s is not a file", binary)
	}
	if pin != "" {
		sum, err := warden.FileSHA256(f)
		if err != nil {
			return "", err
		}
		if !strings.EqualFold(sum, pin) {
			return "", fmt.Errorf("checksum %s of %s does not match --sha256 %s", sum, binary, pin)
		}
	}
	abs, err := filepath.Abs(binary)
	if err != nil {
		return "", err
	}
	return abs, nil
}

func printInstall(out io.Writer, r warden.InstallResult, v warden.Variant) {
	color := terminalColor(out)
	verb := "Installed"
	if r.Updated {
		verb = "Updated"
	}
	fmt.Fprintln(out, paint(fmt.Sprintf("%s %s as %s", verb, v.Name, r.Name), ansiGreen, color))
	fmt.Fprintf(out, "┌ %s\n", paint(r.Name, ansiCyan, color))
	fmt.Fprintf(out, "├ Binary: %s\n", r.Binary)
	fmt.Fprintf(out, "├ Data: %s\n", r.DataDir)
	fmt.Fprintf(out, "├ Config: %s\n", r.ConfigPath)
	if r.TeamKeyFingerprint != "" {
		fmt.Fprintf(out, "├ Team key: %s\n", r.TeamKeyFingerprint)
	}
	fmt.Fprintf(out, "├ Allow from: %s\n", r.AllowFrom)
	fmt.Fprintf(out, "├ Ban: %s  Lock: %s\n", r.Ban, r.Lock)
	if len(r.SafeAccounts) > 0 {
		fmt.Fprintf(out, "├ Safe accounts: %s\n", strings.Join(r.SafeAccounts, ", "))
	}
	fmt.Fprintf(out, "└ Peers: %d\n", len(r.Peers))
	for _, w := range r.Warnings {
		fmt.Fprintln(out, paint("Warning: "+w, ansiYellow, color))
	}
	fmt.Fprintf(out, "Access: ssh -i <team-key> %s@<box> status --json\n", r.Name)
	fmt.Fprintf(out, "Next: configure the box, then run '%s arm' and pair it with a peer.\n", r.Name)
}
