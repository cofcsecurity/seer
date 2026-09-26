package ssh

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func testKeyBlob(kind, payload string) string {
	blob := make([]byte, 4+len(kind)+len(payload))
	binary.BigEndian.PutUint32(blob[:4], uint32(len(kind)))
	copy(blob[4:], kind)
	copy(blob[4+len(kind):], payload)
	return base64.StdEncoding.EncodeToString(blob)
}

func TestParseKeyWithOptions(t *testing.T) {
	blob := testKeyBlob("ssh-ed25519", "test public key blob")
	line := `from="192.0.2.1",command="echo ssh-ed25519 dGVzdA==" ssh-ed25519 ` + blob + ` operator`
	key, ok := parseKey(line, Account{Name: "alice"}, "/home/alice/.ssh/authorized_keys", 4)
	if !ok || key.User != "alice" || key.Line != 4 || key.Type != "ssh-ed25519" || key.Comment != "operator" || key.Fingerprint != fingerprintForBlob(blob) || key.Options != `from="192.0.2.1",command="echo ssh-ed25519 dGVzdA=="` {
		t.Fatalf("unexpected parsed key: %+v (ok=%t)", key, ok)
	}
}

func fingerprintForBlob(encoded string) string {
	blob, _ := base64.StdEncoding.DecodeString(encoded)
	sum := sha256.Sum256(blob)
	return base64.RawStdEncoding.EncodeToString(sum[:])
}

func TestParseKeySkipsCommentsAndWrongBlobType(t *testing.T) {
	blob := testKeyBlob("ssh-ed25519", "test public key blob")
	for _, line := range []string{
		"  # ssh-ed25519 " + blob + " disabled",
		"ssh-rsa " + blob + " mismatched",
		`command="unterminated ssh-ed25519 ` + blob,
	} {
		if key, ok := parseKey(line, Account{Name: "alice"}, "authorized_keys", 1); ok {
			t.Fatalf("parsed non-key line %q as %+v", line, key)
		}
	}
}

func TestRemoveKeyNeedsVerifiedConfig(t *testing.T) {
	_, err := RemoveKey(AuthorizedKey{User: "alice"}, filepath.Join(t.TempDir(), "missing-sshd-config"), "")
	if err == nil || !strings.Contains(err.Error(), "cannot verify authorized key paths") {
		t.Fatalf("expected config verification error, got %v", err)
	}
}

func TestRemoveKeyPreservesOtherEntriesAndBacksUp(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "authorized_keys")
	one := "ssh-ed25519 " + testKeyBlob("ssh-ed25519", "first") + " first"
	two := "ssh-ed25519 " + testKeyBlob("ssh-ed25519", "second") + " second"
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
