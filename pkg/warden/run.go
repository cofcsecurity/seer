package warden

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
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
}

// Args returns the `install` command line, always with --json.
func (o InstallOptions) Args() []string {
	args := []string{"install", "--json"}
	add := func(flag, value string) {
		if value != "" {
			args = append(args, flag, value)
		}
	}
	add("--key", o.Key)
	add("--from", o.From)
	add("--name", o.Name)
	add("--ban", o.Ban)
	add("--lock", o.Lock)
	add("--safe-accounts", strings.Join(o.SafeAccounts, ","))
	if o.Yes {
		args = append(args, "--yes")
	}
	return args
}

// ValidMode reports whether m is a ban or lock mode Warden accepts.
func ValidMode(m string) bool {
	return m == "" || m == "off" || m == "flag" || m == "act"
}

// InstallResult is the JSON `warden install --json` prints.
type InstallResult struct {
	Name               string   `json:"name"`
	Binary             string   `json:"binary"`
	DataDir            string   `json:"data_dir"`
	ConfigPath         string   `json:"config_path"`
	TeamKeyFingerprint string   `json:"team_key_fingerprint,omitempty"`
	AllowFrom          string   `json:"allow_from"`
	Ban                string   `json:"ban"`
	Lock               string   `json:"lock"`
	SafeAccounts       []string `json:"safe_accounts,omitempty"`
	Peers              []string `json:"peers"`
	Updated            bool     `json:"updated"`
	Warnings           []string `json:"warnings,omitempty"`
}

// Run runs a Warden binary with stdin and stderr passed through and returns
// its stdout. Warden reports failures on stderr with exit status 1.
func Run(binary string, args []string, stdin io.Reader, stderr io.Writer) ([]byte, error) {
	var stdout bytes.Buffer
	cmd := exec.Command(binary, args...)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = stdin, &stdout, stderr
	if err := cmd.Run(); err != nil {
		if exit, ok := err.(*exec.ExitError); ok {
			return stdout.Bytes(), fmt.Errorf("%s %s exited with status %d (see the message above)", binary, args[0], exit.ExitCode())
		}
		return stdout.Bytes(), fmt.Errorf("running %s: %w", binary, err)
	}
	return stdout.Bytes(), nil
}

// ParseInstall decodes install JSON, ignoring fields added later.
func ParseInstall(data []byte) (InstallResult, error) {
	var r InstallResult
	if err := json.Unmarshal(data, &r); err != nil {
		return r, fmt.Errorf("warden install printed unexpected output: %w", err)
	}
	if r.Name == "" {
		return r, fmt.Errorf("warden install output has no name")
	}
	return r, nil
}
