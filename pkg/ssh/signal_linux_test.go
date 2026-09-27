//go:build linux && (amd64 || arm64)

package ssh

import (
	"errors"
	"net"
	"net/netip"
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"
)

func TestProcessHasOtherTCPConnections(t *testing.T) {
	root := t.TempDir()
	writeProcessFixture(t, root, 100, 1, 1000, "/usr/sbin/sshd", 111)
	dir := filepath.Join(root, "100/net")
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	data := "sl local_address rem_address st tx_queue rx_queue tr uid timeout inode\n" +
		"0: 0100007F:0016 00000000:0000 0A 0:0 00:0 0 0 0 111\n"
	if err := os.WriteFile(filepath.Join(dir, "tcp"), []byte(data), 0600); err != nil {
		t.Fatal(err)
	}
	p, err := readProcess(root, 100)
	if err != nil {
		t.Fatal(err)
	}
	other, err := processHasOtherTCPConnections(root, p, 222)
	if err != nil || !other {
		t.Fatalf("other TCP connection check = %t, %v", other, err)
	}
}

func TestProcessSignalFallbackRequiresSoleEstablishedSocket(t *testing.T) {
	root := t.TempDir()
	writeProcessFixture(t, root, 100, 1, 1000, "/usr/sbin/sshd", 111, 222)
	dir := filepath.Join(root, "100/net")
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	table := "sl local_address rem_address st tx_queue rx_queue tr uid timeout inode\n" +
		"0: 0100007F:0016 0200007F:ABCD 01 0:0 00:0 0 0 0 111\n" +
		"1: 0100007F:0016 0300007F:ABCD 01 0:0 00:0 0 0 0 222\n"
	if err := os.WriteFile(filepath.Join(dir, "tcp"), []byte(table), 0600); err != nil {
		t.Fatal(err)
	}
	p, err := readProcess(root, 100)
	if err != nil {
		t.Fatal(err)
	}
	if other, err := processHasOtherTCPConnections(root, p, 111); err != nil || !other {
		t.Fatalf("shared process check = %t, %v", other, err)
	}
	p.sockets[222] = false
	if other, err := processHasOtherTCPConnections(root, p, 111); err != nil || other {
		t.Fatalf("sole connection check = %t, %v", other, err)
	}
	if other, err := processHasOtherTCPConnections(root, p, 333); err == nil || other {
		t.Fatalf("missing target check = %t, %v", other, err)
	}
}

func TestShutdownVerifiedSocket(t *testing.T) {
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	client, err := net.Dial("tcp4", listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	server, err := listener.Accept()
	if err != nil {
		t.Fatal(err)
	}
	defer server.Close()
	file, err := server.(*net.TCPConn).File()
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	var stat syscall.Stat_t
	if err := syscall.Fstat(int(file.Fd()), &stat); err != nil {
		t.Fatal(err)
	}
	pidfd, _, errno := syscall.Syscall(sysPIDFDOpen, uintptr(os.Getpid()), 0, 0)
	if errno == syscall.ENOSYS {
		t.Skip("pidfd is unavailable on this kernel")
	}
	if errno != 0 {
		t.Fatal(errno)
	}
	defer syscall.Close(int(pidfd))
	local, err := netip.ParseAddrPort(server.LocalAddr().String())
	if err != nil {
		t.Fatal(err)
	}
	remote, err := netip.ParseAddrPort(server.RemoteAddr().String())
	if err != nil {
		t.Fatal(err)
	}
	target := Session{PID: os.Getpid(), SocketInode: stat.Ino, Local: local, Remote: remote}
	if err := shutdownVerifiedSocket(int(pidfd), target); err != nil {
		if errors.Is(err, syscall.ENOSYS) || errors.Is(err, syscall.EPERM) || errors.Is(err, syscall.EACCES) {
			t.Skipf("pidfd_getfd is unavailable: %v", err)
		}
		t.Fatal(err)
	}
	if err := client.SetReadDeadline(time.Now().Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	var one [1]byte
	if n, err := client.Read(one[:]); n != 0 || err == nil || errors.Is(err, os.ErrDeadlineExceeded) {
		t.Fatalf("socket remained readable after shutdown: n=%d err=%v", n, err)
	}
}
