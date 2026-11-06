package warden

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// GenerateTeamKey makes an ed25519 team keypair in dir with the ssh-keygen
// that ships with OpenSSH, and returns the public key line and the private
// key. The files are left in dir; the caller removes dir, so the private key
// is never kept on the box.
func GenerateTeamKey(dir, comment string) (pub, priv string, err error) {
	keygen, err := exec.LookPath("ssh-keygen")
	if err != nil {
		return "", "", errors.New("--generate-key needs ssh-keygen (part of OpenSSH), which is not installed; give the team key with --key instead")
	}
	file := filepath.Join(dir, "team")
	out, err := exec.Command(keygen, "-q", "-t", "ed25519", "-N", "", "-C", comment, "-f", file).CombinedOutput()
	if err != nil {
		return "", "", fmt.Errorf("ssh-keygen failed: %s", strings.TrimSpace(string(out)))
	}
	pubData, err := os.ReadFile(file + ".pub")
	if err != nil {
		return "", "", err
	}
	privData, err := os.ReadFile(file)
	if err != nil {
		return "", "", err
	}
	line, err := parseTeamKey(string(pubData), "generated key")
	if err != nil {
		return "", "", err
	}
	return line, string(privData), nil
}
