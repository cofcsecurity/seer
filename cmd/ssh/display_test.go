package ssh

import (
	"seer/pkg/ssh"
	"strings"
	"testing"
)

func TestCurrentSessionColorKeepsTextMarker(t *testing.T) {
	session := ssh.Session{ID: "example", Current: true}
	plain := sessionOutput(session, false, false)
	colored := sessionOutput(session, false, true)
	if !strings.Contains(plain, "[CURRENT SESSION]") || strings.Contains(plain, "\x1b[") {
		t.Fatalf("unexpected plain output: %q", plain)
	}
	if !strings.HasPrefix(colored, "\x1b[1;33m") || !strings.HasSuffix(colored, "\x1b[0m\n") || !strings.Contains(colored, "[CURRENT SESSION]") {
		t.Fatalf("unexpected colored output: %q", colored)
	}
	session.Current = false
	if got := sessionOutput(session, false, true); strings.Contains(got, "\x1b[") {
		t.Fatalf("other session was colored: %q", got)
	}
}
