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
	"time"
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
	// Kind is the source type: crontab, cron.d, user, periodic, or anacron.
	Kind Kind
	// ReadOnly jobs come from sources Seer lists but does not rewrite.
	ReadOnly bool
	// Note explains why a job is inactive or read-only, when it is.
	Note string
	// Risks lists reasons an enabled root job's command may be tampered with.
	Risks []string
}

type Kind string

const (
	KindCrontab  Kind = "crontab"
	KindCronD    Kind = "cron.d"
	KindUser     Kind = "user"
	KindPeriodic Kind = "periodic"
	KindAnacron  Kind = "anacron"
)

// DisabledMarker prefixes a line that Seer commented out. Only lines
// carrying it are listed as disabled jobs, so ordinary comments that happen
// to look like cron entries are not mistaken for jobs.
const DisabledMarker = "#[seer-disabled]"

func (j Job) String() string {
	return j.StringStyled(nil)
}

func (j Job) Describe() string {
	return j.DescribeStyled(nil)
}

// StringStyled renders the one-line form, passing each part through style
// (nil leaves the text untouched) so callers can color by role.
func (j Job) StringStyled(style Style) string {
	disabled := ""
	if !j.Enabled {
		disabled = " " + style.apply(RoleDisabled, "[DISABLED]")
	}

	risk := ""
	if len(j.Risks) > 0 {
		risk = " " + style.apply(RoleRisk, "[RISK]")
	}

	if j.ReadOnly {
		risk += " " + style.apply(RoleReadOnly, "["+string(j.Kind)+"]")
	}

	return fmt.Sprintf("[%s] %s %s %s%s\n",
		style.apply(RoleID, j.ID),
		style.apply(j.ScheduleRole(), j.Schedule),
		style.apply(j.UserRole(), j.User),
		style.apply(RoleCommand, fmt.Sprintf("%q", j.Command)),
		disabled+risk,
	)
}

// DescribeStyled renders the detailed form with the same styling hook.
func (j Job) DescribeStyled(style Style) string {
	disabled := ""
	if !j.Enabled {
		disabled = " " + style.apply(RoleDisabled, "[DISABLED]")
	}

	schedule := j.Schedule
	if human := j.humanSchedule(); human != "" {
		schedule = fmt.Sprintf("%s (%s)", j.Schedule, human)
	}

	source := j.Source
	if j.LineNumber > 0 {
		source = fmt.Sprintf("%s (line %d)", j.Source, j.LineNumber)
	}

	kind := string(j.Kind)
	if j.ReadOnly {
		kind += ", read-only"
	}

	var b strings.Builder
	fmt.Fprintf(&b, "┌ %s%s\n", style.apply(RoleID, j.ID), disabled)
	fmt.Fprintf(&b, "├ Type: %s\n", kind)
	fmt.Fprintf(&b, "├ Source: %s\n", source)
	fmt.Fprintf(&b, "├ User: %s\n", style.apply(j.UserRole(), j.User))
	fmt.Fprintf(&b, "├ Schedule: %s\n", style.apply(j.ScheduleRole(), schedule))
	if next := j.nextRunText(); next != "" {
		fmt.Fprintf(&b, "├ Next run: %s\n", next)
	}
	fmt.Fprintf(&b, "├ Command: %s\n", style.apply(RoleCommand, j.Command))
	if j.Note != "" {
		fmt.Fprintf(&b, "├ Note: %s\n", j.Note)
	}
	for _, risk := range j.Risks {
		fmt.Fprintf(&b, "├ %s\n", style.apply(RoleRisk, "Risk: "+risk))
	}
	fmt.Fprintf(&b, "└ Raw: %s\n", j.Raw)

	return b.String()
}

var now = time.Now

// humanSchedule explains the schedule in words. Periodic scripts run at a
// time chosen by cron or anacron, so no clock time is claimed for them.
func (j Job) humanSchedule() string {
	if j.Kind == KindPeriodic {
		return "run via run-parts " + strings.TrimPrefix(j.Schedule, "@") + "; exact time set by cron or anacron"
	}

	return DescribeSchedule(j.Schedule)
}

// nextRunText returns the next fire time for editable cron jobs.
func (j Job) nextRunText() string {
	if j.ReadOnly {
		return ""
	}
	if !j.Enabled {
		return "never (disabled)"
	}

	current := now()
	next, ok := NextRun(j.Schedule, current)
	if !ok {
		return ""
	}

	return fmt.Sprintf("%s (in %s)", next.Format("2006-01-02 15:04"), humanDuration(next.Sub(current)))
}

func humanDuration(d time.Duration) string {
	d = d.Round(time.Minute)
	days, hours, minutes := int(d.Hours())/24, int(d.Hours())%24, int(d.Minutes())%60

	switch {
	case days > 0:
		return fmt.Sprintf("%dd %dh", days, hours)
	case hours > 0:
		return fmt.Sprintf("%dh %dm", hours, minutes)
	default:
		return fmt.Sprintf("%dm", minutes)
	}
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
// Three-letter month and weekday names, such as JAN and MON, are accepted.
var scheduleFieldPattern = regexp.MustCompile(
	`^(?i)(\*|[0-9]+|[a-z]{3})(-([0-9]+|[a-z]{3}))?(/[0-9]+)?` +
		`(,(\*|[0-9]+|[a-z]{3})(-([0-9]+|[a-z]{3}))?(/[0-9]+)?)*$`,
)

// Scanned locations. They are variables so tests can point them at temp dirs.
var (
	crontabPath    = "/etc/crontab"
	cronDDir       = "/etc/cron.d"
	anacrontabPath = "/etc/anacrontab"
	spoolRoots     = []string{"/var/spool/cron", "/var/spool/cron/crontabs"}
)

// idText is the text a job's ID is derived from: the line with any Seer
// disabled marker removed, so a job keeps the same ID when it is disabled and
// enabled again.
func idText(raw string) string {
	line := strings.TrimSpace(raw)
	if rest, ok := strings.CutPrefix(line, DisabledMarker); ok {
		line = strings.TrimSpace(rest)
	}

	return line
}

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

// splitDisabled strips Seer's disabled marker. Any other comment returns an
// empty line so the caller skips it.
func splitDisabled(line string) (rest string, enabled bool) {
	if !strings.HasPrefix(line, "#") {
		return line, true
	}
	if !strings.HasPrefix(line, DisabledMarker) {
		return "", false
	}

	return strings.TrimSpace(strings.TrimPrefix(line, DisabledMarker)), false
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
	line, enabled := splitDisabled(strings.TrimSpace(raw))
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
	line, enabled := splitDisabled(strings.TrimSpace(raw))
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
func parseFile(path string, systemFormat bool, spoolUser string, kind Kind) (jobs []Job, warnings []string) {
	file, err := os.Open(path)
	if err != nil {
		return nil, []string{fmt.Sprintf("could not read %s: %v", path, err)}
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)

	// Increase the default scanner limit for long cron commands.
	scanner.Buffer(make([]byte, 1024), 1024*1024)

	lineNumber := 0
	seen := make(map[string]int)

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

		// Identical lines in one file would otherwise share an ID, making
		// the second one unreachable by enable/disable/remove.
		key := idText(raw)
		id := jobID(path, key)
		if n := seen[id]; n > 0 {
			id = jobID(path, fmt.Sprintf("%s\x00%d", key, n))
		}
		seen[jobID(path, key)]++

		jobs = append(jobs, Job{
			ID:         id,
			Source:     path,
			User:       user,
			Schedule:   schedule,
			Command:    command,
			LineNumber: lineNumber,
			Raw:        raw,
			Enabled:    enabled,
			Kind:       kind,
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

		// Seer's own backups and staging files live beside the source files
		// and must not be mistaken for crontabs.
		if info.Mode().IsRegular() && !strings.HasPrefix(entry.Name(), ".seer-job-") {
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
//	/etc/cron.{hourly,daily,weekly,monthly}
//	/etc/anacrontab
//
// The periodic scripts and anacrontab entries are listed read-only. It parses system crontabs using the format that includes a username and
// parses per-user crontabs using the format where the username comes from
// the filename. A source it cannot read is reported as a warning rather
// than aborting the rest of the scan. Callers are responsible for deciding
// how to surface returned warnings — e.g. printed to the user in `list`,
// but reasonably discarded during shell tab-completion.
func ListJobs() (jobs []Job, warnings []string) {
	if fileExists(crontabPath) {
		fileJobs, fileWarnings := parseFile(crontabPath, true, "", KindCrontab)
		jobs = append(jobs, fileJobs...)
		warnings = append(warnings, fileWarnings...)
	}

	if dirExists(cronDDir) {
		files, dirWarnings := regularFiles(cronDDir)
		warnings = append(warnings, dirWarnings...)

		for _, path := range files {
			fileJobs, fileWarnings := parseFile(path, true, "", KindCronD)
			if reason := cronDIgnored(filepath.Base(path)); reason != "" {
				for i := range fileJobs {
					fileJobs[i].Enabled = false
					fileJobs[i].ReadOnly = true
					fileJobs[i].Note = reason
				}
			}
			jobs = append(jobs, fileJobs...)
			warnings = append(warnings, fileWarnings...)
		}
	}

	// Both common spool locations are scanned, with paths deduplicated
	// because /var/spool/cron/crontabs may be nested below /var/spool/cron.
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

			fileJobs, fileWarnings := parseFile(path, false, user, KindUser)
			jobs = append(jobs, fileJobs...)
			warnings = append(warnings, fileWarnings...)
		}
	}

	periodic, periodicWarnings := periodicJobs()
	jobs = append(jobs, periodic...)
	warnings = append(warnings, periodicWarnings...)

	anacron, anacronWarnings := anacronJobs(anacrontabPath)
	jobs = append(jobs, anacron...)
	warnings = append(warnings, anacronWarnings...)

	for i := range jobs {
		jobs[i].Risks = jobRisks(jobs[i])
	}

	return jobs, warnings
}
