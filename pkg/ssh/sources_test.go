package ssh

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAuthSourcesFromValuesShowsFilesAndExternalProviders(t *testing.T) {
	home := t.TempDir()
	sshDir := filepath.Join(home, ".ssh")
	if err := os.Mkdir(sshDir, 0700); err != nil {
		t.Fatal(err)
	}
	key := "ssh-ed25519 " + testKeyBlob("ssh-ed25519", "test key") + " test\n"
	caPath := filepath.Join(home, "trusted-ca.pub")
	principalPath := filepath.Join(sshDir, "principals")
	hostPath := filepath.Join(home, "ssh_host_ed25519_key")
	for path, content := range map[string]string{
		filepath.Join(sshDir, "authorized_keys"): key,
		caPath:                                   key, principalPath: "# comment\nalice\n",
		hostPath: "private test fixture", hostPath + ".pub": key,
	} {
		if err := os.WriteFile(path, []byte(content), 0600); err != nil {
			t.Fatal(err)
		}
	}
	values := map[string][]string{
		"authorizedkeysfile":        {".ssh/authorized_keys"},
		"authorizedkeyscommand":     {"/usr/local/bin/lookup-key %u"},
		"authorizedkeyscommanduser": {"nobody"},
		"trustedusercakeys":         {caPath},
		"authorizedprincipalsfile":  {".ssh/principals"},
		"hostkey":                   {hostPath},
		"strictmodes":               {"yes"},
	}
	report, err := authSourcesFromValues(Account{Name: "alice", UID: "1000", Home: home}, values)
	if err != nil {
		t.Fatal(err)
	}
	output := report.String()
	for _, want := range []string{
		"AuthorizedKeysFile: " + filepath.Join(sshDir, "authorized_keys"),
		"AuthorizedKeysCommand: /usr/local/bin/lookup-key %u",
		"TrustedUserCAKeys: " + caPath,
		"AuthorizedPrincipalsFile: " + principalPath,
		"Principal: alice",
		"HostKey: " + hostPath,
		"StrictModes: yes",
		"SHA256:" + fingerprintForBlob(testKeyBlob("ssh-ed25519", "test key")),
	} {
		if !strings.Contains(output, want) {
			t.Errorf("source output lacks %q: %s", want, output)
		}
	}
}
