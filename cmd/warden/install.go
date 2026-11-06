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
	var jsonOut, generateKey, password bool
	var roleNames []string
	install := &cobra.Command{
		Use:   "install (--key <team-pub | @file | https://url> | --generate-key)",
		Short: "Obtain and install Warden or Quiet Warden on this box",
		Long: "Obtain Warden and run its install on this box. By default Seer clones the repository and\n" +
			"builds it with Go; --release downloads a GitHub release and verifies it against the\n" +
			"release's SHA256SUMS; --source builds an existing clone; --binary uses a binary already here.\n\n" +
			"Use --variant quiet-warden for Quiet Warden, which is a private repository: Seer checks\n" +
			"access before downloading and explains what to fix when the token is missing or lacks access.\n\n" +
			"Team access: give the team's public key with --key, or let Seer make the keypair here with\n" +
			"--generate-key and print the private key once (it is not kept on this box). --password adds\n" +
			"a team password as a second way in, so the team can log in without the key file.\n\n" +
			"--role adds the important files for this box's role to Warden's watch list after installing\n" +
			"(see 'seer warden profile --help'); --role alone picks roles from what is on the box.\n\n" +
			"Installing again updates the existing install in place. Record the printed name: it is the\n" +
			"binary, the access account, the unit prefix, and the data directory.",
		Example: "  sudo seer warden install --generate-key --password --from 10.0.0.0/24 --role --yes\n" +
			"  sudo --preserve-env=GITHUB_TOKEN seer warden install --key @team.pub --from 10.0.0.0/24 --yes\n" +
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
			if generateKey && opts.Key != "" {
				return errors.New("use --key or --generate-key, not both")
			}
			if n := countTrue(password, opts.PasswordHash != "", opts.NoPassword); n > 1 {
				return errors.New("use only one of --password, --password-hash, and --no-password")
			}
			if opts.PasswordHash != "" {
				if err := warden.CheckPasswordHash(opts.PasswordHash); err != nil {
					return err
				}
				opts.PasswordHash = strings.TrimSpace(opts.PasswordHash)
			}
			if err := warden.CheckRoles(roleNames); err != nil {
				return err
			}
			if opts.Key == "" && !generateKey {
				if found, _ := warden.Find(); len(found) == 0 {
					return errors.New("the team key is required on first install: give it with --key (public key, @file, or https URL),\n" +
						"or use --generate-key to have Seer make one here and print the private key for the team")
				}
			} else if opts.Key != "" {
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

			// Ask for the password and make the key before the slow clone and
			// build, so the team is not left waiting at a prompt.
			if password {
				plain, err := warden.PromptPassword()
				if err != nil {
					return err
				}
				if opts.PasswordHash, err = warden.HashPassword(plain); err != nil {
					return err
				}
			}
			var privateKey string
			if generateKey {
				host, _ := os.Hostname()
				if opts.Key, privateKey, err = warden.GenerateTeamKey(work, "team@"+host); err != nil {
					return err
				}
				fmt.Fprintln(log, "==> Generated a team key (ed25519)")
			}

			bin, err := obtain(cmd.Context(), v, release, ref, source, binary, pin, work, log)
			if err != nil {
				return err
			}

			fmt.Fprintf(log, "==> Installing %s\n", v.Name)
			// Warden prints the install name, how to connect, and next steps.
			// With --json, stdout is kept for the status JSON alone.
			wardenOut := cmd.OutOrStdout()
			if jsonOut {
				wardenOut = log
			}
			if err := warden.RunTo(bin, opts.Args(), cmd.InOrStdin(), wardenOut, log); err != nil {
				return err
			}
			if privateKey != "" {
				printPrivateKey(wardenOut, privateKey, terminalColor(wardenOut))
			}

			in, err := warden.Resolve(opts.Name)
			if err != nil {
				return fmt.Errorf("installed, but Seer could not find the install to check it: %w", err)
			}
			if cmd.Flags().Changed("role") {
				fmt.Fprintf(log, "==> Adding role files to %s\n", in.ProfilePath())
				if err := applyProfile(cmd, in, roleNames, opts.Yes, false); err != nil {
					return fmt.Errorf("installed, but the role profile was not applied: %w\nRun it again with: seer warden profile --role %s", err, strings.Join(roleNames, ","))
				}
			}
			fmt.Fprintf(log, "==> Checking %s\n", in.Name)
			return reportStatus(cmd, in, jsonOut)
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
	f.BoolVar(&generateKey, "generate-key", false, "make the team keypair here and print the private key once, instead of --key")
	f.BoolVar(&password, "password", false, "prompt for a team password to allow as a second way in")
	f.StringVar(&opts.PasswordHash, "password-hash", "", "set the team password from a crypt hash ($6$..., from: openssl passwd -6)")
	f.BoolVar(&opts.NoPassword, "no-password", false, "remove the team password; the key is the only way in")
	f.StringSliceVar(&roleNames, "role", nil, "add role files to the watch list: auto (the default when given alone), "+strings.Join(warden.RoleNames(), ", "))
	f.Lookup("role").NoOptDefVal = "auto"
	f.StringVar(&opts.From, "from", "", `team network: IPs/CIDRs, comma-separated, or "any" (default: the SSH client's /24)`)
	f.StringVar(&opts.Name, "name", "", "install name (default: Warden picks one that blends in)")
	f.StringVar(&opts.Ban, "ban", "", "IP ban response: off, flag, or act (default flag)")
	f.StringVar(&opts.Lock, "lock", "", "account lock response: off, flag, or act (default flag)")
	f.StringSliceVar(&opts.SafeAccounts, "safe-accounts", nil, "accounts Warden never locks (team and scoring accounts)")
	f.BoolVarP(&opts.Yes, "yes", "y", false, "do not prompt; accept defaults")
	f.BoolVar(&opts.NoBreakGlass, "no-break-glass", false, "skip Warden's second, independent access account")
	f.BoolVar(&jsonOut, "json", false, "print the new install's status JSON (Warden's install text goes to stderr)")
	_ = install.RegisterFlagCompletionFunc("role", func(*cobra.Command, []string, string) ([]string, cobra.ShellCompDirective) {
		return append([]string{"auto"}, warden.RoleNames()...), cobra.ShellCompDirectiveNoFileComp
	})
	_ = install.RegisterFlagCompletionFunc("variant", func(*cobra.Command, []string, string) ([]string, cobra.ShellCompDirective) {
		return warden.VariantNames(), cobra.ShellCompDirectiveNoFileComp
	})
	return install
}

func countTrue(values ...bool) int {
	n := 0
	for _, v := range values {
		if v {
			n++
		}
	}
	return n
}

// printPrivateKey shows the generated team key once, with how to use it.
func printPrivateKey(out io.Writer, key string, color bool) {
	fmt.Fprintln(out)
	fmt.Fprintln(out, paint("==> Team private key: shown once, and not kept on this box", ansiYellow, color))
	fmt.Fprintln(out, "Save it on the jump box as 'team' (copy every line, BEGIN to END), then run: chmod 600 team")
	fmt.Fprint(out, key)
	if !strings.HasSuffix(key, "\n") {
		fmt.Fprintln(out)
	}
	fmt.Fprintln(out, "Use the same key on every box: install the others with --key and this box's public key")
	fmt.Fprintln(out, "(ssh-keygen -y -f team > team.pub on the jump box).")
	fmt.Fprintln(out)
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
