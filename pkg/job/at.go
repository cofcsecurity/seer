package job

import (
	"fmt"
	"os"
	"os/user"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Spool directories used by at(1), depending on the distribution.
var atSpoolDirs = []string{"/var/spool/cron/atjobs", "/var/spool/at", "/var/spool/atjobs"}

// atName matches an at spool file: a queue letter, the job number in 5 hex
// digits, and the run time in minutes since the epoch in 8 hex digits.
var atName = regexp.MustCompile(`^([a-zA-Z])([0-9a-f]{5})([0-9a-f]{8})$`)

// isAtPath reports whether path is inside an at spool, which must not be
// mistaken for a per-user crontab when it sits below /var/spool/cron.
func isAtPath(path string) bool {
	for _, dir := range atSpoolDirs {
		if path == dir || strings.HasPrefix(path, dir+string(filepath.Separator)) {
			return true
		}
	}

	return false
}

// atJobs lists pending at(1) jobs from the spool directories.
func atJobs() (jobs []Job, warnings []string) {
	for _, dir := range atSpoolDirs {
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}

		for _, entry := range entries {
			m := atName.FindStringSubmatch(entry.Name())
			if m == nil || m[1] == "=" {
				continue
			}

			path := filepath.Join(dir, entry.Name())
			info, err := entry.Info()
			if err != nil || !info.Mode().IsRegular() {
				continue
			}

			data, err := os.ReadFile(path)
			if err != nil {
				warnings = append(warnings, fmt.Sprintf("could not read %s: %v", path, err))
				continue
			}

			number, _ := strconv.ParseInt(m[2], 16, 64)
			minutes, _ := strconv.ParseInt(m[3], 16, 64)
			runAt := time.Unix(minutes*60, 0)

			jobs = append(jobs, Job{
				ID: jobID(path, entry.Name()), Source: path, User: atOwner(path, string(data)),
				Schedule: "at " + runAt.Format("2006-01-02 15:04"), Command: atCommand(string(data)),
				Raw: path, Enabled: true, Kind: KindAt, ReadOnly: true, Unit: strconv.FormatInt(number, 10),
				Modified: info.ModTime(),
				Note:     "at job " + strconv.FormatInt(number, 10) + "; remove it with 'atrm " + strconv.FormatInt(number, 10) + "'",
			})
		}
	}

	sort.Slice(jobs, func(i, j int) bool { return jobs[i].Schedule < jobs[j].Schedule })

	return jobs, warnings
}

var atrunLine = regexp.MustCompile(`(?m)^# atrun uid=(\d+)`)

// atOwner finds the account an at job runs as, from its header or file owner.
func atOwner(path, script string) string {
	uid := ""
	if m := atrunLine.FindStringSubmatch(script); m != nil {
		uid = m[1]
	} else if info, err := os.Stat(path); err == nil {
		uid = ownerUID(info)
	}
	if uid == "" {
		return "unknown"
	}
	if u, err := user.LookupId(uid); err == nil {
		return u.Username
	}

	return "uid " + uid
}

// atCommand pulls the user's commands out of an at spool script, which is a
// generated preamble (environment, umask, a cd block) followed by the commands.
func atCommand(script string) string {
	lines := strings.Split(script, "\n")

	start := 0
	for i, line := range lines {
		if strings.HasPrefix(line, "cd ") && strings.HasSuffix(strings.TrimSpace(line), "{") {
			for j := i + 1; j < len(lines); j++ {
				if strings.TrimSpace(lines[j]) == "}" {
					start = j + 1
					break
				}
			}
			break
		}
	}

	var cmds []string
	for _, line := range lines[start:] {
		if line = strings.TrimSpace(line); line != "" {
			cmds = append(cmds, line)
		}
	}
	if start == 0 && len(cmds) > 1 {
		cmds = cmds[len(cmds)-1:]
	}

	return strings.Join(cmds, "; ")
}
