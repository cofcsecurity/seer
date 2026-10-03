package job

import (
	"bufio"
	"fmt"
	"hash/fnv"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

type Job struct {
	ID         string
	Source     string
	User       string
	Schedule   string
	Command    string
	LineNumber int
	Raw        string
	Enabled    bool
}

func (j Job) String() string {
	disabled := ""
	if !j.Enabled {
		disabled = " [DISABLED]"
	}

	return fmt.Sprintf("[%s] %s %s %q%s\n", j.ID, j.Schedule, j.User, j.Command, disabled)
}

func (j Job) Describe() string {
	disabled := ""
	if !j.Enabled {
		disabled = " [DISABLED]"
	}

	desc := "┌ %s%s\n"
	desc += "├ Source: %s (line %d)\n"
	desc += "├ User: %s\n"
	desc += "├ Schedule: %s\n"
	desc += "├ Command: %s\n"
	desc += "└ Raw: %s\n"

	return fmt.Sprintf(desc,
		j.ID, disabled,
		j.Source, j.LineNumber,
		j.User,
		j.Schedule,
		j.Command,
		j.Raw,
	)
}

var scheduleMacros = map[string]bool{
	"@reboot":   true,
	"@yearly":   true,
	"@annually": true,
	"@monthly":  true,
	"@weekly":   true,
	"@daily":    true,
	"@midnight": true,
	"@hourly":   true,
}

// scheduleFieldPattern matches standard numeric cron fields:
//
//	*
//	digits
//	ranges
//	steps
//	comma-separated lists
//
// Named months and weekdays, such as JAN and MON, are not recognized.
var scheduleFieldPattern = regexp.MustCompile(
	`^(\*|[0-9]+)(-[0-9]+)?(/[0-9]+)?(,(\*|[0-9]+)(-[0-9]+)?(/[0-9]+)?)*$`,
)

func jobID(source, raw string) string {
	h := fnv.New64a()
	_, _ = h.Write([]byte(source + raw))

	return fmt.Sprintf("%016x", h.Sum64())
}

func validSchedule(fields []string) bool {
	if len(fields) == 1 {
		return scheduleMacros[fields[0]]
	}

	for _, field := range fields {
		if !scheduleFieldPattern.MatchString(field) {
			return false
		}
	}

	return true
}

// parseCrontabLine parses the 6-field /etc/crontab and /etc/cron.d format:
//
//	minute hour day-of-month month day-of-week user command
func parseCrontabLine(raw string) (
	schedule string,
	user string,
	command string,
	enabled bool,
	ok bool,
) {
	line := strings.TrimSpace(raw)
	if line == "" {
		return "", "", "", false, false
	}

	enabled = true

	// Treat commented-out cron entries as disabled jobs.
	if strings.HasPrefix(line, "#") {
		enabled = false
		line = strings.TrimSpace(strings.TrimPrefix(line, "#"))
	}

	if line == "" {
		return "", "", "", false, false
	}

	scheduleFields := 5
	if strings.HasPrefix(line, "@") {
		scheduleFields = 1
	}

	scheduleParts, rest, ok := takeFields(line, scheduleFields)
	if !ok || !validSchedule(scheduleParts) {
		return "", "", "", false, false
	}

	userParts, command, ok := takeFields(rest, 1)
	if !ok {
		return "", "", "", false, false
	}

	schedule = strings.Join(scheduleParts, " ")
	user = userParts[0]

	return schedule, user, command, enabled, true
}

// parseSpoolLine parses the 5-field per-user crontab format:
//
//	minute hour day-of-month month day-of-week command
//
// The user is obtained from the crontab filename rather than the line.
func parseSpoolLine(raw string) (
	schedule string,
	command string,
	enabled bool,
	ok bool,
) {
	line := strings.TrimSpace(raw)
	if line == "" {
		return "", "", false, false
	}

	enabled = true

	// Treat commented-out cron entries as disabled jobs.
	if strings.HasPrefix(line, "#") {
		enabled = false
		line = strings.TrimSpace(strings.TrimPrefix(line, "#"))
	}

	if line == "" {
		return "", "", false, false
	}

	scheduleFields := 5
	if strings.HasPrefix(line, "@") {
		scheduleFields = 1
	}

	scheduleParts, command, ok := takeFields(line, scheduleFields)
	if !ok || !validSchedule(scheduleParts) {
		return "", "", false, false
	}

	schedule = strings.Join(scheduleParts, " ")

	return schedule, command, enabled, true
}

func takeFields(raw string, n int) (
	fields []string,
	rest string,
	ok bool,
) {
	all := strings.Fields(raw)

	// There must be at least n fields plus a command/rest field.
	if len(all) <= n {
		return nil, "", false
	}

	fields = all[:n]
	rest = strings.Join(all[n:], " ")

	return fields, rest, true
}

// parseFile reads path line by line, parsing each line as a cron job. A
// file that can't be opened, or a line that can't be scanned, is reported
// as a warning rather than discarding jobs already found elsewhere.
func parseFile(path string, systemFormat bool, spoolUser string) (jobs []Job, warnings []string) {
	file, err := os.Open(path)
	if err != nil {
		return nil, []string{fmt.Sprintf("could not read %s: %v", path, err)}
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)

	// Increase the default scanner limit for long cron commands.
	scanner.Buffer(make([]byte, 1024), 1024*1024)

	lineNumber := 0

	for scanner.Scan() {
		lineNumber++
		raw := scanner.Text()

		var (
			schedule string
			user     string
			command  string
			enabled  bool
			ok       bool
		)

		if systemFormat {
			schedule, user, command, enabled, ok = parseCrontabLine(raw)
		} else {
			schedule, command, enabled, ok = parseSpoolLine(raw)
			user = spoolUser
		}

		if !ok {
			continue
		}

		jobs = append(jobs, Job{
			ID:         jobID(path, raw),
			Source:     path,
			User:       user,
			Schedule:   schedule,
			Command:    command,
			LineNumber: lineNumber,
			Raw:        raw,
			Enabled:    enabled,
		})
	}

	if err := scanner.Err(); err != nil {
		warnings = append(warnings, fmt.Sprintf("could not finish reading %s: %v", path, err))
	}

	return jobs, warnings
}

// regularFiles recursively returns the paths of all regular files below
// root. A path that can't be read or stat'd is reported as a warning
// rather than aborting the rest of the walk.
func regularFiles(root string) (files []string, warnings []string) {
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			warnings = append(warnings, fmt.Sprintf("could not read %s: %v", path, err))
			return nil
		}

		if entry.IsDir() {
			return nil
		}

		info, err := entry.Info()
		if err != nil {
			warnings = append(warnings, fmt.Sprintf("could not stat %s: %v", path, err))
			return nil
		}

		if info.Mode().IsRegular() {
			files = append(files, path)
		}

		return nil
	})
	if err != nil {
		warnings = append(warnings, fmt.Sprintf("could not walk %s: %v", root, err))
	}

	sort.Strings(files)

	return files, warnings
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.Mode().IsRegular()
}

func dirExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

// ListJobs reads:
//
//	/etc/crontab
//	/etc/cron.d
//	/var/spool/cron
//	/var/spool/cron/crontabs
//
// It parses system crontabs using the format that includes a username and
// parses per-user crontabs using the format where the username comes from
// the filename. A source it cannot read is reported as a warning rather
// than aborting the rest of the scan. Callers are responsible for deciding
// how to surface returned warnings — e.g. printed to the user in `list`,
// but reasonably discarded during shell tab-completion.
func ListJobs() (jobs []Job, warnings []string) {
	if fileExists("/etc/crontab") {
		fileJobs, fileWarnings := parseFile("/etc/crontab", true, "")
		jobs = append(jobs, fileJobs...)
		warnings = append(warnings, fileWarnings...)
	}

	if dirExists("/etc/cron.d") {
		files, dirWarnings := regularFiles("/etc/cron.d")
		warnings = append(warnings, dirWarnings...)

		for _, path := range files {
			fileJobs, fileWarnings := parseFile(path, true, "")
			jobs = append(jobs, fileJobs...)
			warnings = append(warnings, fileWarnings...)
		}
	}

	// Use both common spool locations, but deduplicate paths because
	// /var/spool/cron/crontabs may be nested below /var/spool/cron.
	spoolRoots := []string{
		"/var/spool/cron",
		"/var/spool/cron/crontabs",
	}

	seenFiles := make(map[string]bool)

	for _, root := range spoolRoots {
		if !dirExists(root) {
			continue
		}

		files, dirWarnings := regularFiles(root)
		warnings = append(warnings, dirWarnings...)

		for _, path := range files {
			if seenFiles[path] {
				continue
			}
			seenFiles[path] = true

			user := filepath.Base(path)

			fileJobs, fileWarnings := parseFile(path, false, user)
			jobs = append(jobs, fileJobs...)
			warnings = append(warnings, fileWarnings...)
		}
	}

	return jobs, warnings
}
