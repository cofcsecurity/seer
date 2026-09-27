package ssh

import (
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

func systemCommand(name string) string {
	for _, dir := range []string{"/usr/bin", "/bin", "/usr/sbin", "/sbin"} {
		path := filepath.Join(dir, name)
		if info, err := os.Stat(path); err == nil && info.Mode().IsRegular() && info.Mode()&0111 != 0 {
			return path
		}
	}
	return ""
}

func liveDescendant(root string, child, ancestor int) bool {
	seen := make(map[int]bool)
	for child > 0 && !seen[child] {
		if child == ancestor {
			return true
		}
		seen[child] = true
		p, err := readProcess(root, child)
		if err != nil {
			return false
		}
		child = p.ppid
	}
	return false
}

func addSource(s *Session, source string) {
	for _, existing := range s.Sources {
		if existing == source {
			return
		}
	}
	s.Sources = append(s.Sources, source)
}

func enrichSessions(sessions []Session) {
	if len(sessions) == 0 {
		return
	}
	if loginctl := systemCommand("loginctl"); loginctl != "" {
		output, err := exec.Command(loginctl, "list-sessions", "--no-legend", "--no-pager").Output()
		if err == nil {
			for _, line := range strings.Split(string(output), "\n") {
				fields := strings.Fields(line)
				if len(fields) == 0 {
					continue
				}
				details, err := exec.Command(loginctl, "show-session", fields[0],
					"--property=Leader", "--property=Name", "--property=Remote",
					"--property=Timestamp", "--no-pager").Output()
				if err != nil {
					continue
				}
				values := make(map[string]string)
				for _, item := range strings.Split(string(details), "\n") {
					key, value, ok := strings.Cut(item, "=")
					if ok {
						values[key] = value
					}
				}
				if values["Remote"] != "yes" {
					continue
				}
				leader, err := strconv.Atoi(values["Leader"])
				if err != nil || leader <= 0 {
					continue
				}
				for i := range sessions {
					s := &sessions[i]
					if s.Direction == "inbound" && liveDescendant("/proc", leader, s.PID) {
						if values["Name"] != "" {
							s.User = values["Name"]
						}
						s.LoginTime = values["Timestamp"]
						s.Authenticated = true
						addSource(s, "systemd-logind")
					}
				}
			}
		}
	}
	// who reads the host's utmp/utmpx provider. It is supplementary because
	// not all SSH activity creates a login record (for example, commands
	// without a TTY).
	if who := systemCommand("who"); who != "" {
		output, err := exec.Command(who, "-u").Output()
		if err == nil {
			for _, line := range strings.Split(string(output), "\n") {
				fields := strings.Fields(line)
				if len(fields) < 6 {
					continue
				}
				loginPID, err := strconv.Atoi(fields[5])
				if err != nil || loginPID <= 0 {
					continue
				}
				for i := range sessions {
					s := &sessions[i]
					if s.Direction == "inbound" && liveDescendant("/proc", loginPID, s.PID) {
						s.User = fields[0]
						if s.LoginTime == "" {
							s.LoginTime = fields[2] + " " + fields[3]
						}
						s.Authenticated = true
						addSource(s, "utmp (who)")
					}
				}
			}
		}
	}
}
