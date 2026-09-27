package ssh

import (
	"net/netip"
	"testing"
)

func TestSameKillableTransportRechecksOwnershipAndEndpoints(t *testing.T) {
	target := Session{
		ID: "session", Direction: "inbound", ListenerMatch: true, SharedOwners: 1,
		Local:  netip.MustParseAddrPort("192.0.2.20:22"),
		Remote: netip.MustParseAddrPort("192.0.2.10:50000"),
	}
	if !sameKillableTransport(target, target) {
		t.Fatal("unchanged transport was rejected")
	}
	changed := target
	changed.SharedOwners = 2
	if sameKillableTransport(target, changed) {
		t.Fatal("shared socket was accepted")
	}
	changed = target
	changed.ListenerMatch = false
	if !sameKillableTransport(target, changed) {
		t.Fatal("transport without an sshd-owned listener was rejected")
	}
	changed = target
	changed.Remote = netip.MustParseAddrPort("192.0.2.11:50000")
	if sameKillableTransport(target, changed) {
		t.Fatal("changed endpoint was accepted")
	}
}

func TestSSHContextPresent(t *testing.T) {
	for _, name := range []string{"SSH_CONNECTION", "SSH_CLIENT", "SSH_TTY"} {
		t.Setenv(name, "")
	}
	if SSHContextPresent() {
		t.Fatal("empty SSH environment reported an SSH context")
	}
	for _, name := range []string{"SSH_CONNECTION", "SSH_CLIENT", "SSH_TTY"} {
		t.Setenv(name, "set")
		if !SSHContextPresent() {
			t.Errorf("%s did not report an SSH context", name)
		}
		t.Setenv(name, "")
	}
}
