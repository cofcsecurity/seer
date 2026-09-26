package ssh

import (
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
)

func TestFailedSSHMessage(t *testing.T) {
	for _, test := range []struct {
		line string
		want bool
	}{
		{"sshd[10]: Failed password for invalid user bob from 192.0.2.10", true},
		{"sshd[11]: Failed publickey for alice from 192.0.2.11", true},
		{"sshd[12]: Invalid user guest from 192.0.2.12", true},
		{"sshd[13]: pam_unix(sshd:auth): authentication failure", true},
		{"sshd[14]: maximum authentication attempts exceeded for root", true},
		{"sshd[15]: Accepted publickey for alice from 192.0.2.15", false},
		{"2026-09-25 failed sshd[15]: Accepted publickey for alice", false},
		{"sshd[16]: pam_unix(sshd:session): session opened for user alice", false},
	} {
		if got := failedSSHMessage(test.line); got != test.want {
			t.Errorf("failedSSHMessage(%q) = %t, want %t", test.line, got, test.want)
		}
	}
}

func TestAuthLogTailFiltersBeforeLimit(t *testing.T) {
	path := filepath.Join(t.TempDir(), "auth.log")
	data := strings.Join([]string{
		"sshd[1]: Failed password for bob from 192.0.2.1",
		"sshd[2]: Accepted publickey for alice from 192.0.2.2",
		"cron[3]: authentication failure for cron job",
		"sshd-session[4]: Invalid user guest from 192.0.2.4",
		"sshd[5]: Failed publickey for root from 192.0.2.5",
	}, "\n") + "\n"
	if err := os.WriteFile(path, []byte(data), 0600); err != nil {
		t.Fatal(err)
	}
	got := authLogTail(path, 2, true)
	want := []string{
		"sshd-session[4]: Invalid user guest from 192.0.2.4",
		"sshd[5]: Failed publickey for root from 192.0.2.5",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("failed auth log tail = %q, want %q", got, want)
	}
}

func TestJournalEventsFiltersBeforeLimit(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("requires a POSIX shell")
	}
	path := filepath.Join(t.TempDir(), "journalctl")
	script := "#!/bin/sh\n" +
		"printf '%s\\n' 'sshd[5]: Accepted publickey for alice' 'sshd[4]: Failed password for bob' 'sshd[3]: Accepted password for alice' 'sshd[2]: Invalid user guest' 'sshd[1]: Failed publickey for root'\n"
	if err := os.WriteFile(path, []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	got := journalEvents(path, 2, true)
	want := []string{"sshd[2]: Invalid user guest", "sshd[4]: Failed password for bob"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("failed journal events = %q, want %q", got, want)
	}
}
