//go:build linux && (amd64 || arm64)

package ssh

import (
	"errors"
	"fmt"
	"net/netip"
	"os"
	"path/filepath"
	"strconv"
	"syscall"
)

// Linux pidfd syscalls have these numbers on amd64 and arm64.
const (
	sysPIDFDSendSignal = 424
	sysPIDFDOpen       = 434
	sysPIDFDGetFD      = 438
)

func signalVerifiedSession(target Session) error {
	fd, _, errno := syscall.Syscall(sysPIDFDOpen, uintptr(target.PID), 0, 0)
	if errno != 0 {
		return fmt.Errorf("could not pin SSH process with pidfd: %w", errno)
	}
	defer syscall.Close(int(fd))
	p, err := readProcess("/proc", target.PID)
	if err != nil || p.startTime != target.StartTime || p.netNS != target.NetNS ||
		!p.sockets[target.SocketInode] || sshProcess(p) != "inbound" {
		return fmt.Errorf("SSH process changed; connection left open")
	}
	current, err := listSessions("/proc", 0, "")
	if err != nil {
		return err
	}
	found := false
	for _, session := range current {
		if sameKillableTransport(target, session) {
			found = true
			break
		}
	}
	if !found {
		return fmt.Errorf("SSH connection changed; connection left open")
	}
	err = shutdownVerifiedSocket(int(fd), target)
	if err == nil {
		return nil
	}
	if target.SharedOwners != 1 ||
		(!errors.Is(err, syscall.ENOSYS) && !errors.Is(err, syscall.EPERM) && !errors.Is(err, syscall.EACCES)) {
		return fmt.Errorf("could not safely close SSH socket: %w", err)
	}
	otherConnections, checkErr := processHasOtherTCPConnections("/proc", p, target.SocketInode)
	if checkErr != nil || otherConnections {
		return fmt.Errorf("could not verify a safe SIGTERM fallback; connection left open")
	}
	_, _, errno = syscall.Syscall6(sysPIDFDSendSignal, fd, uintptr(syscall.SIGTERM), 0, 0, 0, 0)
	if errno != 0 {
		return fmt.Errorf("failed to end SSH session: %w", errno)
	}
	return nil
}

func shutdownVerifiedSocket(pidfd int, target Session) error {
	fdNumber, err := socketFDNumber("/proc", target.PID, target.SocketInode)
	if err != nil {
		return err
	}
	duplicate, _, errno := syscall.Syscall6(sysPIDFDGetFD, uintptr(pidfd), uintptr(fdNumber), 0, 0, 0, 0)
	if errno != 0 {
		return errno
	}
	defer syscall.Close(int(duplicate))
	var stat syscall.Stat_t
	if err := syscall.Fstat(int(duplicate), &stat); err != nil || stat.Ino != target.SocketInode {
		return fmt.Errorf("SSH socket descriptor changed")
	}
	local, err := syscall.Getsockname(int(duplicate))
	if err != nil {
		return err
	}
	remote, err := syscall.Getpeername(int(duplicate))
	if err != nil {
		return err
	}
	localAddr, localOK := socketAddress(local)
	remoteAddr, remoteOK := socketAddress(remote)
	if !localOK || !remoteOK || !sameSocketAddress(localAddr, target.Local) || !sameSocketAddress(remoteAddr, target.Remote) {
		return fmt.Errorf("SSH socket endpoints changed")
	}
	return syscall.Shutdown(int(duplicate), syscall.SHUT_RDWR)
}

func socketFDNumber(root string, pid int, inode uint64) (int, error) {
	dir := filepath.Join(root, strconv.Itoa(pid), "fd")
	entries, err := os.ReadDir(dir)
	if err != nil {
		return 0, err
	}
	want := fmt.Sprintf("socket:[%d]", inode)
	for _, entry := range entries {
		target, err := os.Readlink(filepath.Join(dir, entry.Name()))
		if err == nil && target == want {
			return strconv.Atoi(entry.Name())
		}
	}
	return 0, fmt.Errorf("SSH socket descriptor is no longer visible")
}

func socketAddress(value syscall.Sockaddr) (netip.AddrPort, bool) {
	switch addr := value.(type) {
	case *syscall.SockaddrInet4:
		return netip.AddrPortFrom(netip.AddrFrom4(addr.Addr), uint16(addr.Port)), true
	case *syscall.SockaddrInet6:
		return netip.AddrPortFrom(netip.AddrFrom16(addr.Addr), uint16(addr.Port)), true
	}
	return netip.AddrPort{}, false
}

func sameSocketAddress(a, b netip.AddrPort) bool {
	return a.Port() == b.Port() && a.Addr().Unmap() == b.Addr().Unmap()
}

// A process-wide signal is safe only when this is its sole established TCP
// connection and it has no listener. Other open sockets do not affect this
// check; unix sockets are common for sshd's monitor communication.
func processHasOtherTCPConnections(root string, p process, targetInode uint64) (bool, error) {
	base := filepath.Join(root, strconv.Itoa(p.pid), "net")
	readAny, foundTarget := false, false
	for _, table := range []struct {
		name string
		ipv6 bool
	}{{"tcp", false}, {"tcp6", true}} {
		data, err := os.ReadFile(filepath.Join(base, table.name))
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return false, err
		}
		readAny = true
		for _, socket := range parseSocketTable(data, table.ipv6, p.netNS) {
			if p.sockets[socket.inode] && socket.inode == targetInode && socket.state == "01" {
				foundTarget = true
			}
			if p.sockets[socket.inode] && (socket.state == "0A" ||
				(socket.state == "01" && socket.inode != targetInode)) {
				return true, nil
			}
		}
	}
	if !readAny {
		return false, fmt.Errorf("no TCP socket table was readable")
	}
	if !foundTarget {
		return false, fmt.Errorf("selected SSH socket is no longer established")
	}
	return false, nil
}
