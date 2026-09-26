package ssh

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestConfigRulesIncludesSourceLines(t *testing.T) {
	dir := t.TempDir()
	conf := filepath.Join(dir, "sshd_config")
	included := filepath.Join(dir, "extra.conf")
	if err := os.WriteFile(included, []byte("PasswordAuthentication no\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(conf, []byte("# comment\nInclude "+included+"\nPermitRootLogin no\n"), 0600); err != nil {
		t.Fatal(err)
	}
	rules, err := ConfigRules(conf)
	if err != nil {
		t.Fatal(err)
	}
	if len(rules) != 3 || rules[1].Path != included || rules[1].Line != 1 || rules[1].Key != "PasswordAuthentication" {
		t.Fatalf("unexpected rules: %+v", rules)
	}
}

func TestConfigRulesShowsRepeatedInclude(t *testing.T) {
	dir := t.TempDir()
	conf := filepath.Join(dir, "sshd_config")
	included := filepath.Join(dir, "extra.conf")
	if err := os.WriteFile(included, []byte("PasswordAuthentication no\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(conf, []byte("Include "+included+"\nMatch User alice\nInclude "+included+"\n"), 0600); err != nil {
		t.Fatal(err)
	}
	rules, err := ConfigRules(conf)
	if err != nil {
		t.Fatal(err)
	}
	if len(rules) != 5 || rules[1].Path != included || rules[4].Path != included {
		t.Fatalf("repeated Include was omitted: %+v", rules)
	}
}

func TestConfigRulesFollowsQuotedInclude(t *testing.T) {
	dir := t.TempDir()
	conf := filepath.Join(dir, "sshd_config")
	included := filepath.Join(dir, "site rules.conf")
	if err := os.WriteFile(included, []byte("PermitRootLogin no\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(conf, []byte("Include \""+included+"\"\n"), 0600); err != nil {
		t.Fatal(err)
	}
	rules, err := ConfigRules(conf)
	if err != nil {
		t.Fatal(err)
	}
	if len(rules) != 2 || rules[1].Path != included || rules[1].Key != "PermitRootLogin" {
		t.Fatalf("quoted Include was not followed: %+v", rules)
	}
}

func TestConfigRulesRejectsUnterminatedIncludeQuote(t *testing.T) {
	conf := filepath.Join(t.TempDir(), "sshd_config")
	if err := os.WriteFile(conf, []byte("Include \"missing file\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := ConfigRules(conf); err == nil || !strings.Contains(err.Error(), "invalid Include arguments") {
		t.Fatalf("expected quoted Include error, got %v", err)
	}
}

func TestRuntimeArguments(t *testing.T) {
	path, overrides, inetd := runtimeArguments([]byte("/usr/sbin/sshd\x00-D\x00-f\x00/etc/ssh/other.conf\x00-o\x00PasswordAuthentication=no\x00-p2222\x00"))
	if path != "/etc/ssh/other.conf" || inetd || len(overrides) != 2 {
		t.Fatalf("unexpected runtime arguments: path=%q overrides=%q inetd=%t", path, overrides, inetd)
	}
	_, _, inetd = runtimeArguments([]byte("sshd\x00-i\x00"))
	if !inetd {
		t.Fatal("inetd mode was not found")
	}
}

func TestRunningServersFindsListeningSSHD(t *testing.T) {
	root := t.TempDir()
	writeProcessFixture(t, root, 100, 1, 1000, "/usr/sbin/sshd", 111)
	if err := os.WriteFile(filepath.Join(root, "100/cmdline"), []byte("/usr/sbin/sshd\x00-D\x00-f\x00/etc/ssh/site.conf\x00"), 0600); err != nil {
		t.Fatal(err)
	}
	netDir := filepath.Join(root, "100/net")
	if err := os.MkdirAll(netDir, 0700); err != nil {
		t.Fatal(err)
	}
	data := "sl local_address rem_address st tx_queue rx_queue tr uid timeout inode\n" +
		"0: 0100007F:0016 00000000:0000 0A 0:0 00:0 0 0 0 111\n"
	if err := os.WriteFile(filepath.Join(netDir, "tcp"), []byte(data), 0600); err != nil {
		t.Fatal(err)
	}
	servers, err := runningServers(root)
	if err != nil || len(servers) != 1 || servers[0].ConfigPath != "/etc/ssh/site.conf" ||
		len(servers[0].Listeners) != 1 || servers[0].Listeners[0] != "127.0.0.1:22" {
		t.Fatalf("unexpected runtime servers: %+v, %v", servers, err)
	}
}

func TestCheckConfigCoversAuthenticationAndForwarding(t *testing.T) {
	findings := CheckConfig(map[string]string{
		"permitrootlogin": "prohibit-password", "kbdinteractiveauthentication": "yes",
		"allowagentforwarding": "yes", "allowtcpforwarding": "remote", "gatewayports": "clientspecified",
	})
	if len(findings) != 5 {
		t.Fatalf("expected five review findings, got %+v", findings)
	}
}

func TestConfigRulesReportsInvalidInclude(t *testing.T) {
	dir := t.TempDir()
	conf := filepath.Join(dir, "sshd_config")
	if err := os.WriteFile(conf, []byte("Include [unclosed\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := ConfigRules(conf); err == nil || !strings.Contains(err.Error(), "invalid Include pattern") {
		t.Fatalf("expected Include pattern error, got %v", err)
	}
}

func TestConfigRulesReportsUnreadableInclude(t *testing.T) {
	dir := t.TempDir()
	conf := filepath.Join(dir, "sshd_config")
	included := filepath.Join(dir, "included-directory")
	if err := os.Mkdir(included, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(conf, []byte("Include "+included+"\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := ConfigRules(conf); err == nil || !strings.Contains(err.Error(), "could not read Include") {
		t.Fatalf("expected unreadable Include error, got %v", err)
	}
}
