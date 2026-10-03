package job

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

var periodicDirs = []struct {
	dir      string
	schedule string
}{
	{"/etc/cron.hourly", "@hourly"},
	{"/etc/cron.daily", "@daily"},
	{"/etc/cron.weekly", "@weekly"},
	{"/etc/cron.monthly", "@monthly"},
}

// periodicName matches names run-parts executes: letters, digits, underscores
// and hyphens only. Files with dots (such as .dpkg-old) are skipped by cron.
var periodicName = regexp.MustCompile(`^[A-Za-z0-9_-]+$`)

// periodicJobs lists the scripts in /etc/cron.{hourly,daily,weekly,monthly}.
// They run as root via run-parts, so a script is enabled only when its name
// is eligible and it is executable. They are read-only.
func periodicJobs() (jobs []Job, warnings []string) {
	for _, p := range periodicDirs {
		if !dirExists(p.dir) {
			continue
		}

		entries, err := os.ReadDir(p.dir)
		if err != nil {
			warnings = append(warnings, fmt.Sprintf("could not read %s: %v", p.dir, err))
			continue
		}

		for _, entry := range entries {
			// Skip hidden files such as Debian's .placeholder.
			if strings.HasPrefix(entry.Name(), ".") {
				continue
			}

			path := filepath.Join(p.dir, entry.Name())

			info, err := os.Stat(path)
			if err != nil || !info.Mode().IsRegular() {
				continue
			}

			jobs = append(jobs, Job{
				ID:       jobID(path, path),
				Source:   path,
				User:     "root",
				Schedule: p.schedule,
				Command:  path,
				Raw:      path,
				Enabled:  periodicName.MatchString(entry.Name()) && info.Mode()&0o111 != 0,
				Kind:     KindPeriodic,
				ReadOnly: true,
			})
		}
	}

	return jobs, warnings
}

// anacronJobs parses /etc/anacrontab lines of the form
//
//	period delay job-identifier command
//
// where period is a number of days or @daily/@weekly/@monthly.
func anacronJobs(path string) (jobs []Job, warnings []string) {
	if !fileExists(path) {
		return nil, nil
	}

	file, err := os.Open(path)
	if err != nil {
		return nil, []string{fmt.Sprintf("could not read %s: %v", path, err)}
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 1024), 1024*1024)

	lineNumber := 0
	seen := make(map[string]int)

	for scanner.Scan() {
		lineNumber++
		raw := scanner.Text()
		line := strings.TrimSpace(raw)

		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		fields := strings.Fields(line)
		if len(fields) < 4 || strings.Contains(fields[0], "=") {
			continue
		}

		schedule, ok := anacronSchedule(fields[0])
		if !ok {
			continue
		}

		id := jobID(path, raw)
		if n := seen[id]; n > 0 {
			id = jobID(path, fmt.Sprintf("%s\x00%d", raw, n))
		}
		seen[jobID(path, raw)]++

		jobs = append(jobs, Job{
			ID:         id,
			Source:     path,
			User:       "root",
			Schedule:   schedule,
			Command:    strings.Join(fields[3:], " "),
			LineNumber: lineNumber,
			Raw:        raw,
			Enabled:    true,
			Kind:       KindAnacron,
			ReadOnly:   true,
		})
	}

	if err := scanner.Err(); err != nil {
		warnings = append(warnings, fmt.Sprintf("could not finish reading %s: %v", path, err))
	}

	return jobs, warnings
}

// anacronSchedule maps an anacron period to a cron macro where one exists
// and to "@every-Nd" otherwise.
func anacronSchedule(period string) (string, bool) {
	switch period {
	case "@daily", "@weekly", "@monthly", "@yearly", "@annually":
		return period, true
	case "1":
		return "@daily", true
	case "7":
		return "@weekly", true
	case "30", "31":
		return "@monthly", true
	}

	n, err := strconv.Atoi(period)
	if err != nil || n < 1 {
		return "", false
	}

	return fmt.Sprintf("@every-%dd", n), true
}
