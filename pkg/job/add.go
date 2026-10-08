package job

import (
	"bytes"
	"fmt"
	"os"
	"os/user"
	"path/filepath"
	"strings"
)

// AddSpec describes a job to create.
type AddSpec struct {
	User     string
	Schedule string
	Command  string
	File     string // optional; defaults to the user's existing crontab
}

// AddPlan is a validated job addition, ready to preview or apply.
type AddPlan struct {
	Path   string
	Line   string
	Exists bool
	Risks  []string
	system bool
}

// PlanAdd validates spec and works out which file gets which line. A job goes
// in the user's existing crontab by default, or in a file given with File:
// /etc/crontab, a file in /etc/cron.d, or a spool crontab.
func PlanAdd(spec AddSpec) (AddPlan, error) {
	schedule := strings.Join(strings.Fields(spec.Schedule), " ")
	fields := strings.Fields(schedule)

	switch {
	case len(fields) == 0:
		return AddPlan{}, fmt.Errorf("a schedule is required")
	case len(fields) != 1 && len(fields) != 5:
		return AddPlan{}, fmt.Errorf("schedule %q needs five fields (minute hour day month weekday) or a macro such as @daily", spec.Schedule)
	case !validSchedule(fields):
		return AddPlan{}, fmt.Errorf("schedule %q is not a valid cron schedule", spec.Schedule)
	}
	if len(fields) == 5 {
		if _, ok := NextRun(schedule, now()); !ok {
			return AddPlan{}, fmt.Errorf("schedule %q never fires", spec.Schedule)
		}
	}

	command := strings.TrimSpace(spec.Command)
	switch {
	case command == "":
		return AddPlan{}, fmt.Errorf("a command is required")
	case strings.ContainsAny(command, "\r\n"):
		return AddPlan{}, fmt.Errorf("the command must be a single line")
	case hasUnescapedPercent(command):
		return AddPlan{}, fmt.Errorf("cron treats %% as a newline; write \\%% to keep a literal percent sign")
	}

	if spec.User == "" {
		return AddPlan{}, fmt.Errorf("a user is required")
	}
	if _, err := user.Lookup(spec.User); err != nil {
		return AddPlan{}, fmt.Errorf("user %q does not exist", spec.User)
	}

	plan := AddPlan{}
	if err := plan.chooseFile(spec); err != nil {
		return AddPlan{}, err
	}

	if plan.system {
		plan.Line = schedule + " " + spec.User + " " + command
	} else {
		plan.Line = schedule + " " + command
	}

	if info, err := os.Lstat(plan.Path); err == nil {
		if !info.Mode().IsRegular() {
			return AddPlan{}, fmt.Errorf("%s is not a regular file", plan.Path)
		}
		plan.Exists = true

		data, err := os.ReadFile(plan.Path)
		if err != nil {
			return AddPlan{}, err
		}
		for _, line := range strings.Split(string(data), "\n") {
			if strings.TrimRight(line, "\r") == plan.Line {
				return AddPlan{}, fmt.Errorf("%s already contains that job", plan.Path)
			}
		}
	}

	plan.Risks = jobRisks(Job{User: spec.User, Command: command, Enabled: true})

	return plan, nil
}

func hasUnescapedPercent(command string) bool {
	for i := 0; i < len(command); i++ {
		switch command[i] {
		case '\\':
			i++
		case '%':
			return true
		}
	}

	return false
}

func (p *AddPlan) chooseFile(spec AddSpec) error {
	if spec.File == "" {
		for _, root := range spoolRoots {
			path := filepath.Join(root, spec.User)
			if info, err := os.Stat(path); err == nil && info.Mode().IsRegular() {
				p.Path = path
				return nil
			}
		}

		return fmt.Errorf("%s has no crontab file here; create one with 'crontab -u %s -e', or add the job to a system file with --file %s/<name>",
			spec.User, spec.User, cronDDir)
	}

	path := filepath.Clean(spec.File)

	switch {
	case path == crontabPath:
		p.system = true
	case filepath.Dir(path) == cronDDir:
		p.system = true
		if reason := cronDIgnored(filepath.Base(path)); reason != "" {
			return fmt.Errorf("cron would ignore %s: %s", path, reason)
		}
	case isSpoolFile(path):
		if filepath.Base(path) != spec.User {
			return fmt.Errorf("%s belongs to %s, not %s", path, filepath.Base(path), spec.User)
		}
	default:
		return fmt.Errorf("%s is not a cron file; use %s, a file in %s, or a user crontab", path, crontabPath, cronDDir)
	}

	p.Path = path

	return nil
}

func isSpoolFile(path string) bool {
	for _, root := range spoolRoots {
		if filepath.Dir(path) == root && !isAtPath(path) {
			return true
		}
	}

	return false
}

// AddDiff shows the lines an add would write.
func AddDiff(p AddPlan) string {
	if !p.Exists {
		return fmt.Sprintf("%s does not exist; it would be created:\n+%s\n", p.Path, p.Line)
	}

	return fmt.Sprintf("--- %s\n+++ %s (proposed)\n+%s\n", p.Path, p.Path, p.Line)
}

// Add appends the job, saving a backup so it can be undone with Restore. It
// returns the new job's ID and the backup path.
func Add(p AddPlan) (id, backupPath string, err error) {
	dir, base := filepath.Dir(p.Path), filepath.Base(p.Path)

	var original []byte
	var info os.FileInfo
	if p.Exists {
		if info, err = os.Lstat(p.Path); err != nil {
			return "", "", err
		}
		if original, err = os.ReadFile(p.Path); err != nil {
			return "", "", err
		}
	}

	content := append([]byte(nil), original...)
	if len(content) > 0 && !bytes.HasSuffix(content, []byte("\n")) {
		content = append(content, '\n')
	}
	content = append(content, []byte(p.Line+"\n")...)

	backupPath, err = writeBackup(dir, base, string(OpAdd), original)
	if err != nil {
		return "", "", fmt.Errorf("could not create backup: %w", err)
	}

	lines := strings.Split(strings.TrimSuffix(string(original), "\n"), "\n")
	meta := Meta{Targeted: true, Created: !p.Exists, Line: len(lines) + 1, HasNew: true, New: p.Line, Mode: 0o644}
	if info != nil {
		meta = mergeFileMeta(meta, fileMeta(info))
	}
	if len(original) > 0 {
		meta.Before = lines[len(lines)-1]
	}
	_ = writeMeta(backupPath, meta)

	if !p.Exists {
		if err := recreate(p.Path, content, Meta{Mode: 0o644, UID: os.Geteuid(), GID: os.Getegid()}); err != nil {
			return "", backupPath, err
		}
	} else {
		tmp, err := writeReplacement(dir, p.Path, info, content)
		if err != nil {
			return "", backupPath, fmt.Errorf("could not stage replacement: %w", err)
		}
		if err := commitReplacement(p.Path, info, original, tmp); err != nil {
			_ = os.Remove(tmp)
			return "", backupPath, err
		}
	}

	jobs, _ := ListJobs()
	for i := len(jobs) - 1; i >= 0; i-- {
		if jobs[i].Source == p.Path && jobs[i].Raw == p.Line {
			return jobs[i].ID, backupPath, nil
		}
	}

	return "", backupPath, nil
}

func mergeFileMeta(m, f Meta) Meta {
	m.Mode, m.UID, m.GID = f.Mode, f.UID, f.GID

	return m
}
