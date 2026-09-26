package ssh

import (
	"seer/pkg/ssh"
	"testing"
)

func TestMatchingKeysCanNarrowDuplicateFingerprint(t *testing.T) {
	keys := []ssh.AuthorizedKey{
		{User: "alice", Path: "/home/alice/.ssh/authorized_keys", Line: 2, Fingerprint: "abc"},
		{User: "alice", Path: "/home/alice/.ssh/authorized_keys", Line: 5, Fingerprint: "abc"},
		{User: "alice", Path: "/home/alice/.ssh/other_keys", Line: 2, Fingerprint: "abc"},
	}
	if got := matchingKeys(keys, "SHA256:abc", "", 0); len(got) != 3 {
		t.Fatalf("found %d duplicates, want 3", len(got))
	}
	got := matchingKeys(keys, "SHA256:abc", "/home/alice/.ssh/authorized_keys", 5)
	if len(got) != 1 || got[0].Line != 5 {
		t.Fatalf("wrong selected key: %+v", got)
	}
}
