package ssh

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

func failedSSHMessage(line string) bool {
	message := strings.ToLower(line)
	if _, body, ok := strings.Cut(message, "]: "); ok {
		message = body
	}
	return (strings.Contains(message, "failed ") && strings.Contains(message, " for ")) ||
		strings.Contains(message, "invalid user ") ||
		strings.Contains(message, "authentication failure") ||
		strings.Contains(message, "maximum authentication attempts exceeded")
}

func authLogTail(path string, limit int, failed bool) []string {
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
		if (!strings.Contains(line, "sshd[") && !strings.Contains(line, "sshd-session[")) ||
			(failed && !failedSSHMessage(line)) {
			continue
		}
		lines = append(lines, line)
		if len(lines) > limit {
			lines = lines[1:]
		}
	}
	return lines
}

func journalEvents(path string, limit int, failed bool) []string {
	cmd := exec.Command(path, "_COMM=sshd", "_COMM=sshd-session", "+",
		"SYSLOG_IDENTIFIER=sshd", "SYSLOG_IDENTIFIER=sshd-session",
		"-r", "--no-pager", "-o", "short-iso")
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil
	}
	if err := cmd.Start(); err != nil {
		_ = stdout.Close()
		return nil
	}
	scanner := bufio.NewScanner(stdout)
	scanner.Buffer(make([]byte, 4096), 1024*1024)
	var lines []string
	stoppedEarly := false
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "-- ") {
			continue
		}
		if failed && !failedSSHMessage(line) {
			continue
		}
		lines = append(lines, line)
		if len(lines) == limit {
			stoppedEarly = true
			_ = cmd.Process.Kill()
			_ = stdout.Close()
			break
		}
	}
	waitErr := cmd.Wait()
	if scanner.Err() != nil || (!stoppedEarly && waitErr != nil) {
		return nil
	}
	for i, j := 0, len(lines)-1; i < j; i, j = i+1, j-1 {
		lines[i], lines[j] = lines[j], lines[i]
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
	if path := systemCommand("journalctl"); path != "" {
		if lines := journalEvents(path, limit, failed); len(lines) > 0 {
			label := "all types"
			if failed {
				label = "failed authentication"
			}
			sections = append(sections, fmt.Sprintf("SSH daemon journal entries (%s):\n%s\n", label, strings.Join(lines, "\n")))
		}
	}
	for _, path := range []string{"/var/log/auth.log", "/var/log/secure", "/var/log/messages", "/var/log/syslog"} {
		if lines := authLogTail(path, limit, failed); len(lines) > 0 {
			label := "all types"
			if failed {
				label = "failed authentication"
			}
			sections = append(sections, fmt.Sprintf("SSH daemon text log (%s; %s):\n%s\n", path, label, strings.Join(lines, "\n")))
		}
	}
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
	if len(sections) == 0 {
		return "", fmt.Errorf("no matching entries found in available history sources")
	}
	return strings.Join(sections, "\n"), nil
}
