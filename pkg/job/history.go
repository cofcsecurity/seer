package job

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// logFiles are searched when the journal has no matching cron entries.
var logFiles = []string{"/var/log/syslog", "/var/log/cron", "/var/log/cron.log", "/var/log/messages"}

// maxLogBytes limits how much of each log file's tail is read.
const maxLogBytes = 8 << 20

// History is what the system logs show about a job's recent runs.
type History struct {
	Source  string   // where the entries came from
	Entries []string // oldest first
	Note    string   // what the logs cannot tell
}

// JobHistory finds up to limit recent runs of j in the journal or log files.
// Cron logs when it starts a job but not the exit status or output; systemd
// timers log both through their service unit.
func JobHistory(j Job, limit int) (History, error) {
	if limit < 1 {
		limit = 10
	}

	switch j.Kind {
	case KindTimer:
		return timerHistory(j, limit)
	case KindAt:
		return History{}, fmt.Errorf("at jobs run once and leave no history; use 'journalctl -t atd' for the daemon's log")
	}

	return cronHistory(j, limit)
}

func timerHistory(j Job, limit int) (History, error) {
	service := j.Service
	if service == "" {
		service = strings.TrimSuffix(j.Unit, ".timer") + ".service"
	}

	out, err := runCommand("journalctl", "--no-pager", "-o", "short-iso",
		"-n", fmt.Sprint(limit*3), "-u", service, "-u", j.Unit)
	if err != nil {
		return History{}, fmt.Errorf("journalctl failed: %w: %s", err, strings.TrimSpace(string(out)))
	}

	var entries []string
	for _, line := range strings.Split(string(out), "\n") {
		if line = strings.TrimSpace(line); line != "" && !strings.HasPrefix(line, "-- ") {
			entries = append(entries, line)
		}
	}

	return History{
		Source:  "journalctl -u " + service,
		Entries: lastN(entries, limit*3),
		Note:    "the unit's start, exit status, and output are in these lines",
	}, nil
}

func cronHistory(j Job, limit int) (History, error) {
	match := cronMatcher(j)
	h := History{Note: "cron records when it starts a job, not its exit status or output"}

	// Try the journal first, then plain log files.
	if out, err := runCommand("journalctl", "--no-pager", "-o", "short-iso", "-t", "CRON", "-t", "cron", "-t", "crond", "-t", "anacron", "-n", "20000"); err == nil {
		if entries := filterLines(string(out), match); len(entries) > 0 {
			h.Source, h.Entries = "journalctl", lastN(entries, limit)
			return h, nil
		}
	}

	for _, path := range logFiles {
		text, err := tailFile(path, maxLogBytes)
		if err != nil {
			continue
		}
		if entries := filterLines(text, match); len(entries) > 0 {
			h.Source, h.Entries = path, lastN(entries, limit)
			return h, nil
		}
	}

	h.Source = "the journal and " + strings.Join(logFiles, ", ")

	return h, nil
}

// cronMatcher returns a function that recognizes log lines for j.
func cronMatcher(j Job) func(string) bool {
	switch j.Kind {
	case KindPeriodic:
		dir := filepath.Dir(j.Command)
		base := filepath.Base(dir)

		return func(line string) bool {
			return strings.Contains(line, "run-parts") && strings.Contains(line, dir) ||
				strings.Contains(line, "anacron") && strings.Contains(line, base)
		}
	case KindAnacron:
		return func(line string) bool {
			return strings.Contains(line, "anacron") && strings.Contains(line, "`"+j.Unit+"'")
		}
	}

	command := j.Command
	if len(command) > 80 {
		command = command[:80]
	}

	return func(line string) bool {
		return strings.Contains(line, "("+j.User+") CMD (") && strings.Contains(line, command)
	}
}

func filterLines(text string, match func(string) bool) []string {
	var out []string
	for _, line := range strings.Split(text, "\n") {
		if match(line) {
			out = append(out, strings.TrimSpace(line))
		}
	}

	return out
}

func lastN(lines []string, n int) []string {
	if len(lines) > n {
		return lines[len(lines)-n:]
	}

	return lines
}

// tailFile returns up to max bytes from the end of a file.
func tailFile(path string, max int64) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()

	info, err := f.Stat()
	if err != nil {
		return "", err
	}
	if info.Size() > max {
		if _, err := f.Seek(info.Size()-max, io.SeekStart); err != nil {
			return "", err
		}
	}

	data, err := io.ReadAll(f)

	return string(data), err
}
