package ssh

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseAddress(t *testing.T) {
	for _, test := range []struct {
		value string
		ipv6  bool
		want  string
	}{
		{"0100007F:0016", false, "127.0.0.1:22"},
		{"00000000000000000000000001000000:0016", true, "[::1]:22"},
	} {
		got, err := parseAddress(test.value, test.ipv6)
		if err != nil || got.String() != test.want {
			t.Fatalf("parseAddress(%q) = %s, %v; want %s", test.value, got, err, test.want)
		}
	}
}

func TestSessionIDIncludesEndpoints(t *testing.T) {
	p := process{pid: 42, startTime: 100}
	one := socket{inode: 9, netNS: "net:[1]"}
	one.local, _ = parseAddress("0100007F:0016", false)
	one.remote, _ = parseAddress("0200007F:C350", false)
	two := one
	two.remote, _ = parseAddress("0300007F:C350", false)
	if sessionID(p, one) == sessionID(p, two) {
		t.Fatal("session ID did not change with the remote endpoint")
	}
}

func writeProcessFixture(t *testing.T, root string, pid, ppid int, start uint64, exe string, sockets ...int) {
	t.Helper()
	base := filepath.Join(root, fmt.Sprint(pid))
	if err := os.MkdirAll(filepath.Join(base, "fd"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(base, "ns"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("net:[1]", filepath.Join(base, "ns/net")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(exe, filepath.Join(base, "exe")); err != nil {
		t.Fatal(err)
	}
	fields := make([]string, 22)
	for i := range fields {
		fields[i] = "0"
	}
	fields[0], fields[1], fields[19] = "S", fmt.Sprint(ppid), fmt.Sprint(start)
	if err := os.WriteFile(filepath.Join(base, "stat"), []byte(fmt.Sprintf("%d (ssh test) %s", pid, strings.Join(fields, " "))), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(base, "status"), []byte("Uid:\t0\t0\t0\t0\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(base, "cmdline"), []byte(exe+"\x00"), 0600); err != nil {
		t.Fatal(err)
	}
	for i, inode := range sockets {
		if err := os.Symlink(fmt.Sprintf("socket:[%d]", inode), filepath.Join(base, "fd", fmt.Sprint(i))); err != nil {
			t.Fatal(err)
		}
	}
}

func TestListSessionsUsesLiveSocketAndMarksCurrent(t *testing.T) {
	root := t.TempDir()
	writeProcessFixture(t, root, 100, 1, 1000, "/usr/sbin/sshd", 111)
	writeProcessFixture(t, root, 101, 100, 1001, "/usr/sbin/sshd", 222)
	writeProcessFixture(t, root, 102, 101, 1002, "/bin/sh")
	writeProcessFixture(t, root, 200, 102, 2000, "/usr/local/bin/seer")
	if err := os.WriteFile(filepath.Join(root, "102/environ"), []byte("SSH_CONNECTION=127.0.0.2 50000 127.0.0.1 22\x00"), 0600); err != nil {
		t.Fatal(err)
	}
	netDir := filepath.Join(root, "100/net")
	if err := os.MkdirAll(netDir, 0700); err != nil {
		t.Fatal(err)
	}
	data := "sl local_address rem_address st tx_queue rx_queue tr uid timeout inode\n" +
		"0: 0100007F:0016 00000000:0000 0A 0:0 00:0 0 0 0 111\n" +
		"1: 0100007F:0016 0200007F:C350 01 0:0 00:0 0 0 0 222\n"
	if err := os.WriteFile(filepath.Join(netDir, "tcp"), []byte(data), 0600); err != nil {
		t.Fatal(err)
	}
	sessions, err := listSessions(root, 200, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(sessions) != 1 {
		t.Fatalf("got %d sessions, want 1", len(sessions))
	}
	s := sessions[0]
	if !s.Current || !s.Authenticated || !s.ListenerMatch || s.PID != 101 {
		t.Fatalf("unexpected session: %+v", s)
	}
}

func TestListSessionsCollapsesSharedSocketOwners(t *testing.T) {
	root := t.TempDir()
	writeProcessFixture(t, root, 100, 1, 1000, "/usr/sbin/sshd", 111)
	writeProcessFixture(t, root, 101, 100, 1001, "/usr/sbin/sshd", 222)
	writeProcessFixture(t, root, 102, 101, 1002, "/usr/lib/openssh/sshd-session", 222)
	netDir := filepath.Join(root, "100/net")
	if err := os.MkdirAll(netDir, 0700); err != nil {
		t.Fatal(err)
	}
	data := "sl local_address rem_address st tx_queue rx_queue tr uid timeout inode\n" +
		"0: 0100007F:0016 00000000:0000 0A 0:0 00:0 0 0 0 111\n" +
		"1: 0100007F:0016 0200007F:C350 01 0:0 00:0 0 0 0 222\n"
	if err := os.WriteFile(filepath.Join(netDir, "tcp"), []byte(data), 0600); err != nil {
		t.Fatal(err)
	}
	sessions, err := listSessions(root, 102, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(sessions) != 1 || sessions[0].SharedOwners != 2 || !sessions[0].Current {
		t.Fatalf("expected one current session with two owners, got %+v", sessions)
	}
}
