//go:build linux && (amd64 || arm64)

package ssh

import (
	"fmt"
	"syscall"
)

// Linux pidfd syscalls have these numbers on amd64 and arm64.
const (
	sysPIDFDSendSignal = 424
	sysPIDFDOpen       = 434
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
		return fmt.Errorf("SSH process changed; no signal sent")
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
		return fmt.Errorf("SSH connection changed; no signal sent")
	}
	_, _, errno = syscall.Syscall6(sysPIDFDSendSignal, fd, uintptr(syscall.SIGTERM), 0, 0, 0, 0)
	if errno != 0 {
		return fmt.Errorf("failed to end SSH session: %w", errno)
	}
	return nil
}
