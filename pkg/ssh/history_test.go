package ssh

import (
	"compress/gzip"
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

func TestAuthLogTailReadsGzipRotation(t *testing.T) {
	path := filepath.Join(t.TempDir(), "auth.log.2.gz")
	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	compressed := gzip.NewWriter(file)
	if _, err := compressed.Write([]byte("Sep 25 13:40:02 host sshd[10]: Failed password for bob\n")); err != nil {
		t.Fatal(err)
	}
	if err := compressed.Close(); err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	got := authLogTail(path, 10, true)
	if len(got) != 1 || !strings.Contains(got[0], "Failed password") {
		t.Fatalf("gzip history = %q", got)
	}
}

func TestFindAuthLogPathsIncludesOnlyKnownRotations(t *testing.T) {
	base := filepath.Join(t.TempDir(), "auth.log")
	for _, path := range []string{base, base + ".1", base + ".2.gz", base + ".old.gz", base + ".bak"} {
		if err := os.WriteFile(path, []byte(""), 0600); err != nil {
			t.Fatal(err)
		}
	}
	got := findAuthLogPaths([]string{base})
	want := []string{base, base + ".1", base + ".2.gz"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("log paths = %q, want %q", got, want)
	}
}

func TestHistoryDeduplicatesJournalAndTextLogEvent(t *testing.T) {
	journal := "2026-09-25T13:40:02+0000 host sshd[10]: Failed password for bob"
	textLog := "Sep 25 13:40:02 host sshd[10]: Failed password for bob"
	seen := make(map[string]bool)
	if got := newHistoryLines([]string{journal}, seen); len(got) != 1 {
		t.Fatalf("journal event missing: %q", got)
	}
	if got := newHistoryLines([]string{textLog}, seen); len(got) != 0 {
		t.Fatalf("duplicate text event remained: %q", got)
	}
	rfc3339 := "2026-09-25T13:40:02Z host sshd[10]: Failed password for bob"
	if got := newHistoryLines([]string{rfc3339}, seen); len(got) != 0 {
		t.Fatalf("RFC3339 duplicate remained: %q", got)
	}
	if got := newHistoryLines([]string{journal, journal}, make(map[string]bool)); len(got) != 2 {
		t.Fatalf("same-source events were collapsed: %q", got)
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
