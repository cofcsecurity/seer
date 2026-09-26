package ssh

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

func authLogTail(path string, limit int) []string {
	file, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer file.Close()
	var lines []string
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 4096), 1024*1024)
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.Contains(line, "sshd[") && !strings.Contains(line, "sshd-session[") {
			continue
		}
		lines = append(lines, line)
		if len(lines) > limit {
			lines = lines[1:]
		}
	}
	return lines
}

// LoginHistory returns optional host login records and sshd journal entries.
// wtmp/btmp records are system-wide; their presence does not prove SSH use.
func LoginHistory(failed bool, limit int) (string, error) {
	if limit < 1 || limit > 500 {
		return "", fmt.Errorf("limit must be between 1 and 500")
	}
	var sections []string
	name, source := "last", "wtmp"
	if failed {
		name, source = "lastb", "btmp"
	}
	if path := systemCommand(name); path != "" {
		output, err := exec.Command(path, "-n", fmt.Sprint(limit), "-i").CombinedOutput()
		if err == nil && strings.TrimSpace(string(output)) != "" {
			sections = append(sections, fmt.Sprintf("System login records (%s; not SSH-specific):\n%s", source, output))
		}
	}
	if path := systemCommand("journalctl"); path != "" {
		output, err := exec.Command(path, "_COMM=sshd", "_COMM=sshd-session", "-n", fmt.Sprint(limit), "--no-pager", "-o", "short-iso").CombinedOutput()
		if err == nil && strings.TrimSpace(string(output)) != "" {
			sections = append(sections, fmt.Sprintf("SSH daemon journal entries (all types):\n%s", output))
		}
	}
	for _, path := range []string{"/var/log/auth.log", "/var/log/secure", "/var/log/messages", "/var/log/syslog"} {
		if lines := authLogTail(path, limit); len(lines) > 0 {
			sections = append(sections, fmt.Sprintf("SSH daemon text log (%s):\n%s\n", path, strings.Join(lines, "\n")))
		}
	}
	if len(sections) == 0 {
		return "", fmt.Errorf("no readable login history sources found")
	}
	return strings.Join(sections, "\n"), nil
}
