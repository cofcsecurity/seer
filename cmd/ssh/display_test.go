package ssh

import (
	"bytes"
	"os"
	"regexp"
	"seer/pkg/ssh"
	"strings"
	"testing"
)

var ansiCode = regexp.MustCompile("\x1b\\[[0-9;]*m")

func plainText(value string) string {
	return ansiCode.ReplaceAllString(value, "")
}

func TestTerminalColorRespectsNoColorAndRedirects(t *testing.T) {
	t.Setenv("TERM", "xterm-256color")
	t.Setenv("NO_COLOR", "1")
	if terminalColor(os.Stdout) {
		t.Fatal("NO_COLOR did not disable color")
	}
	if err := os.Unsetenv("NO_COLOR"); err != nil {
		t.Fatal(err)
	}
	if terminalColor(&bytes.Buffer{}) {
		t.Fatal("buffer output should remain plain")
	}
	file, err := os.CreateTemp(t.TempDir(), "output")
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	if terminalColor(file) {
		t.Fatal("redirected file output should remain plain")
	}
}

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
	if got := sessionOutput(session, false, true); !strings.Contains(got, ansiCyan) ||
		!strings.Contains(got, ansiYellow) || plainText(got) != session.String() {
		t.Fatalf("other session ID was not highlighted cleanly: %q", got)
	}
	session.Authenticated = true
	if got := sessionOutput(session, false, true); !strings.Contains(got, ansiGreen) || plainText(got) != session.String() {
		t.Fatalf("authenticated status was not highlighted cleanly: %q", got)
	}
}

func TestWarningColorFallsBackToPlainText(t *testing.T) {
	const warning = "WARNING: This is your current SSH connection."
	if got := warningOutput(warning, false); got != warning {
		t.Fatalf("unexpected plain warning: %q", got)
	}
	if got := warningOutput(warning, true); got != "\x1b[1;31m"+warning+"\x1b[0m" {
		t.Fatalf("unexpected red warning: %q", got)
	}
}

func TestSSHColorHighlightsKeepPlainOutput(t *testing.T) {
	cases := []struct {
		name, plain, colored, style string
	}{
		{"config rule", configRuleOutput(ssh.ConfigRule{Path: "/etc/ssh/sshd_config", Line: 3, Key: "Match", Value: "User alice"}, false),
			configRuleOutput(ssh.ConfigRule{Path: "/etc/ssh/sshd_config", Line: 3, Key: "Match", Value: "User alice"}, true), ansiYellow},
		{"warning finding", configFindingOutput(ssh.ConfigFinding{Level: "warning", Key: "permitrootlogin", Value: "yes", Reason: "review"}, false),
			configFindingOutput(ssh.ConfigFinding{Level: "warning", Key: "permitrootlogin", Value: "yes", Reason: "review"}, true), ansiRed},
		{"review finding", configFindingOutput(ssh.ConfigFinding{Level: "review", Key: "passwordauthentication", Value: "yes", Reason: "review"}, false),
			configFindingOutput(ssh.ConfigFinding{Level: "review", Key: "passwordauthentication", Value: "yes", Reason: "review"}, true), ansiYellow},
		{"effective setting", configEffectiveOutput("permitrootlogin", "yes", "warning", false),
			configEffectiveOutput("permitrootlogin", "yes", "warning", true), ansiRed},
		{"key list", keyListOutput(ssh.AuthorizedKey{User: "alice", Fingerprint: "abc", Type: "ssh-ed25519", Path: "/home/alice/.ssh/authorized_keys", Line: 2}, false),
			keyListOutput(ssh.AuthorizedKey{User: "alice", Fingerprint: "abc", Type: "ssh-ed25519", Path: "/home/alice/.ssh/authorized_keys", Line: 2}, true), ansiCyan},
		{"description", labelOutput("┌ alice SHA256:abc\n├ File: /home/alice/.ssh/authorized_keys:2\n", false),
			labelOutput("┌ alice SHA256:abc\n├ File: /home/alice/.ssh/authorized_keys:2\n", true), ansiCyan},
		{"key sources", sourcesOutput("Authentication sources for alice:\nWarning: missing file\n", false),
			sourcesOutput("Authentication sources for alice:\nWarning: missing file\n", true), ansiRed},
		{"history", historyOutput("SSH daemon journal entries (all types):\nsshd[1]: Failed password for alice\nsshd[2]: Accepted publickey for bob\n", false, false),
			historyOutput("SSH daemon journal entries (all types):\nsshd[1]: Failed password for alice\nsshd[2]: Accepted publickey for bob\n", false, true), ansiGreen},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			if strings.Contains(test.plain, "\x1b[") || plainText(test.colored) != test.plain ||
				!strings.Contains(test.colored, test.style) {
				t.Fatalf("plain=%q colored=%q", test.plain, test.colored)
			}
		})
	}
}
