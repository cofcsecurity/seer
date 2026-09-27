package ssh

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

// EndSession closes one identified inbound SSH transport. A caller must
// first display the session and confirm the action.
// currentConfirmed must only be true after two explicit confirmations.
func EndSession(id string, currentConfirmed bool) error {
	parts := strings.Split(id, ":")
	if len(parts) != 4 {
		return fmt.Errorf("invalid session ID")
	}
	pid, err := strconv.Atoi(parts[0])
	if err != nil {
		return fmt.Errorf("invalid session ID")
	}
	start, err := strconv.ParseUint(parts[1], 10, 64)
	if err != nil {
		return fmt.Errorf("invalid session ID")
	}
	inode, err := strconv.ParseUint(parts[2], 10, 64)
	if err != nil {
		return fmt.Errorf("invalid session ID")
	}
	sessions, status, err := ListSessionsWithStatus()
	if err != nil {
		return err
	}
	var target *Session
	for i := range sessions {
		if sessions[i].ID == id {
			target = &sessions[i]
			break
		}
	}
	if target == nil || target.PID != pid || target.StartTime != start || target.SocketInode != inode {
		return fmt.Errorf("session changed or disconnected; list sessions again")
	}
	if target.Direction != "inbound" {
		return fmt.Errorf("cannot verify this is an inbound SSH transport")
	}
	if target.Current && !currentConfirmed {
		return fmt.Errorf("current SSH session requires two explicit confirmations")
	}
	if SSHContextPresent() || status.Incomplete {
		foundCurrent := false
		for _, s := range sessions {
			foundCurrent = foundCurrent || s.Current
		}
		if !foundCurrent && !currentConfirmed {
			return fmt.Errorf("current SSH connection could not be identified; two explicit confirmations required")
		}
	}
	// The process may change between listing and action. Hold a pidfd and
	// verify its start time and socket before closing the transport.
	return signalVerifiedSession(*target)
}

// SSHContextPresent reports whether this process inherited evidence that it
// is running inside an SSH login. It is used when the live connection cannot
// be matched to the current process.
func SSHContextPresent() bool {
	return os.Getenv("SSH_CONNECTION") != "" || os.Getenv("SSH_CLIENT") != "" || os.Getenv("SSH_TTY") != ""
}

func sameKillableTransport(target, current Session) bool {
	return current.ID == target.ID && current.Local == target.Local && current.Remote == target.Remote &&
		current.Direction == "inbound" && current.SharedOwners == target.SharedOwners && current.SharedOwners > 0
}
