package warden

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"hash"
	"io"
	"os/exec"
	"strings"
)

type hasher struct{ hash.Hash }

func newHasher() hasher      { return hasher{sha256.New()} }
func (h hasher) sum() string { return hex.EncodeToString(h.Sum(nil)) }

// FileSHA256 returns the hex SHA-256 of a file's contents.
func FileSHA256(r io.Reader) (string, error) {
	h := newHasher()
	if _, err := io.Copy(h, r); err != nil {
		return "", err
	}
	return h.sum(), nil
}

// InstallOptions are passed through to `warden install`.
type InstallOptions struct {
	Key          string
	From         string
	Name         string
	Ban          string
	Lock         string
	SafeAccounts []string
	Yes          bool
	NoBreakGlass bool
	PasswordHash string
	NoPassword   bool
}

// Args returns the `install` command line. Warden's install prints a plain
// summary and next steps; Seer shows it as is and reads status afterwards.
func (o InstallOptions) Args() []string {
	args := []string{"install"}
	add := func(flag, value string) {
		if value != "" {
			args = append(args, flag, value)
		}
	}
	add("--key", o.Key)
	add("--password-hash", o.PasswordHash)
	add("--from", o.From)
	add("--name", o.Name)
	add("--ban", o.Ban)
	add("--lock", o.Lock)
	add("--safe-accounts", strings.Join(o.SafeAccounts, ","))
	if o.NoPassword {
		args = append(args, "--no-password")
	}
	if o.NoBreakGlass {
		args = append(args, "--no-break-glass")
	}
	if o.Yes {
		args = append(args, "--yes")
	}
	return args
}

// ValidMode reports whether m is a ban or lock mode Warden accepts.
func ValidMode(m string) bool {
	return m == "" || m == "off" || m == "flag" || m == "act"
}

// Run runs a Warden binary with stdin and stderr passed through and returns
// its stdout. Warden reports failures on stderr with exit status 1.
func Run(binary string, args []string, stdin io.Reader, stderr io.Writer) ([]byte, error) {
	var stdout bytes.Buffer
	err := RunTo(binary, args, stdin, &stdout, stderr)
	return stdout.Bytes(), err
}

// RunTo runs a Warden binary with all three streams connected as given.
func RunTo(binary string, args []string, stdin io.Reader, stdout, stderr io.Writer) error {
	cmd := exec.Command(binary, args...)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = stdin, stdout, stderr
	if err := cmd.Run(); err != nil {
		if exit, ok := err.(*exec.ExitError); ok {
			return fmt.Errorf("%s %s exited with status %d (see the message above)", binary, args[0], exit.ExitCode())
		}
		return fmt.Errorf("running %s: %w", binary, err)
	}
	return nil
}
