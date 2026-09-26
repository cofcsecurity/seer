package ssh

import (
	"net/netip"
	"testing"
)

func TestSameKillableTransportRechecksOwnershipAndListener(t *testing.T) {
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
	if sameKillableTransport(target, changed) {
		t.Fatal("unmatched listener was accepted")
	}
	changed = target
	changed.Remote = netip.MustParseAddrPort("192.0.2.11:50000")
	if sameKillableTransport(target, changed) {
		t.Fatal("changed endpoint was accepted")
	}
}
