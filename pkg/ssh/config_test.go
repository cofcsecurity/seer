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
