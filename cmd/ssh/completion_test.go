package ssh

import (
	"strings"
	"testing"

	"seer/pkg/ssh"
)

func TestSessionIDCompletions(t *testing.T) {
	sessions := []ssh.Session{
		{ID: "10:1:2:abc", Direction: "inbound", Current: true},
		{ID: "11:1:3:def", Direction: "outbound"},
	}
	end := sessionIDCompletions(sessions, true, "1")
	if len(end) != 1 || !strings.HasPrefix(end[0], "10:1:2:abc\t") || !strings.Contains(end[0], "CURRENT SESSION") {
		t.Fatalf("end completions = %q", end)
	}
	describe := sessionIDCompletions(sessions, false, "11")
	if len(describe) != 1 || !strings.HasPrefix(describe[0], "11:1:3:def\t") {
		t.Fatalf("describe completions = %q", describe)
	}
}

func TestKeyFingerprintCompletions(t *testing.T) {
	keys := []ssh.AuthorizedKey{
		{User: "alice", Fingerprint: "first", Path: "/home/alice/.ssh/authorized_keys", Line: 1},
		{User: "bob", Fingerprint: "first", Path: "/home/bob/.ssh/authorized_keys", Line: 2},
		{User: "bob", Fingerprint: "second", Path: "/home/bob/.ssh/authorized_keys", Line: 3},
	}
	matches := keyFingerprintCompletions(keys, "SHA256:f")
	if len(matches) != 1 || !strings.HasPrefix(matches[0], "SHA256:first\t") {
		t.Fatalf("key completions = %q", matches)
	}
}
