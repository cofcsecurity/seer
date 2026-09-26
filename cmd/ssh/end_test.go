package ssh

import (
	"bytes"
	"seer/pkg/ssh"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

func TestConfirmCurrentEndRequiresYesTwice(t *testing.T) {
	for _, test := range []struct {
		input string
		want  bool
	}{
		{"yes\nyes\n", true},
		{"yes\nno\n", false},
		{"no\nyes\n", false},
		{"yes\n", false},
	} {
		var out bytes.Buffer
		got := confirmCurrentEnd(strings.NewReader(test.input), &out)
		if got != test.want {
			t.Errorf("confirmCurrentEnd(%q) = %t, want %t", test.input, got, test.want)
		}
	}
}

func TestCurrentSessionYesFlagStillRequiresTwoConfirmations(t *testing.T) {
	t.Setenv("SSH_CONNECTION", "192.0.2.10 52644 192.0.2.20 22")
	const id = "1832:567890:41852:9f62a3b0178a4c20"
	for _, test := range []struct {
		input string
		want  bool
	}{
		{"yes\nno\n", false},
		{"yes\nyes\n", true},
	} {
		var out bytes.Buffer
		cmd := &cobra.Command{}
		cmd.SetIn(strings.NewReader(test.input))
		cmd.SetOut(&out)
		confirmed, risk := confirmSessionEnd(cmd, ssh.Session{ID: id, Current: true}, true, true)
		if confirmed != test.want || !risk {
			t.Errorf("confirmSessionEnd(%q) = (%t, %t), want (%t, true)", test.input, confirmed, risk, test.want)
		}
		if !strings.Contains(out.String(), "WARNING:") || !strings.Contains(out.String(), "Confirm again") {
			t.Errorf("missing warning or second confirmation: %q", out.String())
		}
	}
}

func TestUnknownCurrentConnectionAlsoRequiresTwoConfirmations(t *testing.T) {
	t.Setenv("SSH_CONNECTION", "192.0.2.10 52644 192.0.2.20 22")
	var out bytes.Buffer
	cmd := &cobra.Command{}
	cmd.SetIn(strings.NewReader("no\n"))
	cmd.SetOut(&out)
	confirmed, risk := confirmSessionEnd(cmd, ssh.Session{ID: "other"}, false, true)
	if confirmed || !risk || !strings.Contains(out.String(), "could not be identified") {
		t.Fatalf("unexpected unknown-current result: confirmed=%t risk=%t output=%q", confirmed, risk, out.String())
	}
}
