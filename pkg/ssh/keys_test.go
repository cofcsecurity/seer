package ssh

import (
	"encoding/base64"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseKeyWithOptions(t *testing.T) {
	blob := base64.StdEncoding.EncodeToString([]byte("test public key blob"))
	line := `from="192.0.2.1",command="echo hello" ssh-ed25519 ` + blob + ` operator`
	key, ok := parseKey(line, Account{Name: "alice"}, "/home/alice/.ssh/authorized_keys", 4)
	if !ok || key.User != "alice" || key.Line != 4 || key.Type != "ssh-ed25519" || key.Comment != "operator" || key.Fingerprint == "" {
		t.Fatalf("unexpected parsed key: %+v (ok=%t)", key, ok)
	}
}

func TestRemoveKeyPreservesOtherEntriesAndBacksUp(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "authorized_keys")
	one := "ssh-ed25519 " + base64.StdEncoding.EncodeToString([]byte("first")) + " first"
	two := "ssh-ed25519 " + base64.StdEncoding.EncodeToString([]byte("second")) + " second"
	original := "# keep this comment\n" + one + "\n" + two + "\n"
	if err := os.WriteFile(path, []byte(original), 0600); err != nil {
		t.Fatal(err)
	}
	key, ok := parseKey(one, Account{Name: "alice"}, path, 2)
	if !ok {
		t.Fatal("key was not parsed")
	}
	backup, err := removeKeyFile(key)
	if err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "# keep this comment\n"+two+"\n" {
		t.Fatalf("unexpected file content: %q", got)
	}
	old, err := os.ReadFile(backup)
	if err != nil || string(old) != original {
		t.Fatalf("backup differs: %q, %v", old, err)
	}
	info, err := os.Stat(path)
	if err != nil || info.Mode().Perm() != 0600 {
		t.Fatalf("permissions changed: %v, %v", info, err)
	}
	if _, err := removeKeyFile(key); err == nil || !strings.Contains(err.Error(), "changed") {
		t.Fatalf("stale key removal should fail, got %v", err)
	}
}
