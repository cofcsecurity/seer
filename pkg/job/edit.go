package job

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
)

// locateLine opens path, rejects non-regular files, and finds the job's
// line by lineNumber, falling back to a unique match on expected.
func locateLine(path string, lineNumber int, expected string) (
	lines []string, index int, info os.FileInfo, err error,
) {
	info, err = os.Lstat(path)
	if err != nil {
		return nil, 0, nil, err
	}

	if !info.Mode().IsRegular() {
		return nil, 0, nil, fmt.Errorf("%s is not a regular file", path)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, 0, nil, err
	}

	lines = strings.SplitAfter(string(data), "\n")

	trimmed := func(line string) string {
		line = strings.TrimSuffix(line, "\n")
		return strings.TrimSuffix(line, "\r")
	}

	if lineNumber >= 1 &&
		lineNumber <= len(lines) &&
		trimmed(lines[lineNumber-1]) == expected {
		return lines, lineNumber - 1, info, nil
	}

	matchIndex := -1
	matchCount := 0

	for i, line := range lines {
		if trimmed(line) == expected {
			matchIndex = i
			matchCount++
		}
	}

	if matchCount == 1 {
		return lines, matchIndex, info, nil
	}

	if matchCount == 0 {
		return nil, 0, info, fmt.Errorf(
			"line %d is out of range and expected line %q was not found",
			lineNumber, expected,
		)
	}

	return nil, 0, info, fmt.Errorf(
		"line %d is out of range and expected line %q matched %d lines",
		lineNumber, expected, matchCount,
	)
}

// writeBackup saves data to a new, synced file in dir in case the edit fails
// or needs undoing. The name records the operation and the file it protects
// (".seer-job-backup-<op>-<file>-<random>") so backups can be listed and
// restored later.
func writeBackup(dir, base, op string, data []byte) (backupPath string, err error) {
	backup, err := os.CreateTemp(dir, backupPrefix+op+"-"+base+"-*")
	if err != nil {
		return "", err
	}

	backupPath = backup.Name()

	defer func() {
		if err != nil {
			_ = backup.Close()
			_ = os.Remove(backupPath)
			backupPath = ""
		}
	}()

	if _, err = backup.Write(data); err != nil {
		return "", err
	}

	if err = backup.Sync(); err != nil {
		return "", err
	}

	if err = backup.Close(); err != nil {
		return "", err
	}

	return backupPath, nil
}

// writeReplacement writes content to a new temp file in dir, matching
// info's mode, owner, and extended attributes from original.
func writeReplacement(
	dir, original string,
	info os.FileInfo,
	content []byte,
) (tmpPath string, err error) {
	tmp, err := os.CreateTemp(dir, ".seer-job-*")
	if err != nil {
		return "", err
	}

	tmpPath = tmp.Name()

	defer func() {
		if err != nil {
			_ = tmp.Close()
			_ = os.Remove(tmpPath)
			tmpPath = ""
		}
	}()

	if err = tmp.Chmod(info.Mode().Perm()); err != nil {
		return "", err
	}

	if stat, ok := info.Sys().(*syscall.Stat_t); ok {
		if int(stat.Uid) != os.Geteuid() || int(stat.Gid) != os.Getegid() {
			if err = tmp.Chown(int(stat.Uid), int(stat.Gid)); err != nil {
				return "", err
			}
		}
	}

	if err = copyExtendedAttributes(original, tmpPath); err != nil {
		return "", fmt.Errorf("could not preserve file attributes: %w", err)
	}

	if _, err = tmp.Write(content); err != nil {
		return "", err
	}

	if err = tmp.Sync(); err != nil {
		return "", err
	}

	if err = tmp.Close(); err != nil {
		return "", err
	}

	return tmpPath, nil
}

// commitReplacement confirms path still matches original and is still the
// same file, then renames tmpPath over it.
func commitReplacement(
	path string,
	info os.FileInfo,
	original []byte,
	tmpPath string,
) error {
	currentInfo, err := os.Lstat(path)
	if err != nil {
		return fmt.Errorf("could not re-stat source file: %w", err)
	}

	if !os.SameFile(info, currentInfo) {
		return fmt.Errorf("source file changed before replacement")
	}

	current, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("could not re-read source file: %w", err)
	}

	if !bytes.Equal(current, original) {
		return fmt.Errorf("source file changed before replacement")
	}

	if err := os.Rename(tmpPath, path); err != nil {
		return fmt.Errorf("could not commit replacement: %w", err)
	}

	return nil
}

// rewriteSource locates the line, builds the new content, backs up the
// original, stages the replacement, and commits it.
//
// A nil replacement removes the matching line.
func rewriteSource(op Operation, job Job, replacement *string) (backupPath string, err error) {
	lines, index, info, err := locateLine(job.Source, job.LineNumber, job.Raw)
	if err != nil {
		return "", err
	}

	original := []byte(strings.Join(lines, ""))

	if replacement == nil {
		lines = append(lines[:index], lines[index+1:]...)
	} else {
		lines[index] = *replacement + lineEnding(lines[index])
	}

	content := []byte(strings.Join(lines, ""))
	dir := filepath.Dir(job.Source)

	backupPath, err = writeBackup(dir, filepath.Base(job.Source), string(op), original)
	if err != nil {
		return "", fmt.Errorf("could not create backup: %w", err)
	}

	meta := fileMeta(info)
	meta.Targeted, meta.Line, meta.HasOld, meta.Old = true, index+1, true, job.Raw
	if index > 0 {
		meta.Before = strings.TrimRight(lines[index-1], "\r\n")
	}
	if replacement != nil {
		meta.HasNew, meta.New = true, *replacement
	}
	// Best effort: without the sidecar, restore falls back to the whole file.
	_ = writeMeta(backupPath, meta)

	tmpPath, err := writeReplacement(dir, job.Source, info, content)
	if err != nil {
		return backupPath, fmt.Errorf("could not stage replacement: %w", err)
	}

	if err := commitReplacement(job.Source, info, original, tmpPath); err != nil {
		_ = os.Remove(tmpPath)
		return backupPath, err
	}

	return backupPath, nil
}

func lineEnding(line string) string {
	switch {
	case strings.HasSuffix(line, "\r\n"):
		return "\r\n"
	case strings.HasSuffix(line, "\n"):
		return "\n"
	case strings.HasSuffix(line, "\r"):
		return "\r"
	default:
		return ""
	}
}

// findJob returns the job matching id. If it can't be found and ListJobs
// reported warnings, those are included, since an unreadable source is a
// more useful answer than a bare "not found".
func findJob(id string) (Job, error) {
	jobs, warnings := ListJobs()

	for _, j := range jobs {
		if j.ID == id {
			return j, nil
		}
	}

	if len(warnings) > 0 {
		return Job{}, fmt.Errorf(
			"job %q not found (%d source(s) could not be read: %s)",
			id, len(warnings), strings.Join(warnings, "; "),
		)
	}

	return Job{}, fmt.Errorf("job %q not found", id)
}

// Operation is a change Seer can make to a job's source line.
type Operation string

const (
	OpDisable Operation = "disable"
	OpEnable  Operation = "enable"
	OpRemove  Operation = "remove"
	OpRestore Operation = "restore"
	OpAdd     Operation = "add"
)

// plan finds the job and returns it with its replacement line (nil removes
// the line), rejecting read-only jobs and no-op changes.
func plan(op Operation, id string) (target Job, replacement *string, err error) {
	target, err = findJob(id)
	if err != nil {
		return Job{}, nil, err
	}

	return planFor(op, target)
}

func planFor(op Operation, target Job) (Job, *string, error) {
	id := target.ID

	if target.ReadOnly {
		msg := fmt.Sprintf("job %q comes from %s and is read-only; edit %s directly", id, target.Kind, target.Source)
		if target.Note != "" {
			msg += " (" + target.Note + ")"
		}
		return Job{}, nil, fmt.Errorf("%s", msg)
	}

	switch op {
	case OpDisable:
		if !target.Enabled {
			return Job{}, nil, fmt.Errorf("job %q is already disabled", id)
		}
		line := DisabledMarker + " " + target.Raw
		return target, &line, nil
	case OpEnable:
		if target.Enabled {
			return Job{}, nil, fmt.Errorf("job %q is already enabled", id)
		}
		line := strings.TrimSpace(strings.TrimPrefix(strings.TrimPrefix(strings.TrimSpace(target.Raw), DisabledMarker), "#"))
		return target, &line, nil
	case OpRemove:
		// No enabled/disabled guard: removing a disabled job is valid.
		return target, nil, nil
	}

	return Job{}, nil, fmt.Errorf("unknown operation %q", op)
}

// Apply performs op on the job and returns the path of the backup it saved.
func Apply(op Operation, id string) (backupPath string, err error) {
	target, err := findJob(id)
	if err != nil {
		return "", err
	}

	if target.Kind == KindTimer {
		return "", applyTimer(op, target)
	}

	target, replacement, err := planFor(op, target)
	if err != nil {
		return "", err
	}

	return rewriteSource(op, target, replacement)
}

// Diff shows what Apply would change, without touching any file. The output
// is a small unified-style diff with one line of context on each side.
func Diff(op Operation, id string) (string, error) {
	target, err := findJob(id)
	if err != nil {
		return "", err
	}

	if target.Kind == KindTimer {
		if err := checkTimerOp(op, target); err != nil {
			return "", err
		}
		return "Would run: systemctl " + string(op) + " --now " + target.Unit + "\n", nil
	}

	target, replacement, err := planFor(op, target)
	if err != nil {
		return "", err
	}

	return diffFor(target, replacement)
}

func diffFor(target Job, replacement *string) (string, error) {
	lines, index, _, err := locateLine(target.Source, target.LineNumber, target.Raw)
	if err != nil {
		return "", err
	}

	trim := func(line string) string { return strings.TrimRight(line, "\r\n") }

	var b strings.Builder
	fmt.Fprintf(&b, "--- %s\n+++ %s (proposed)\n@@ line %d @@\n", target.Source, target.Source, index+1)

	if index > 0 {
		fmt.Fprintf(&b, " %s\n", trim(lines[index-1]))
	}
	fmt.Fprintf(&b, "-%s\n", trim(lines[index]))
	if replacement != nil {
		fmt.Fprintf(&b, "+%s\n", *replacement)
	}
	if index+1 < len(lines) && trim(lines[index+1]) != "" {
		fmt.Fprintf(&b, " %s\n", trim(lines[index+1]))
	}

	return b.String(), nil
}

// Disable comments out job's line. Errors if the job is already disabled.
func Disable(id string) (string, error) { return Apply(OpDisable, id) }

// Enable uncomments job's line. Errors if the job is already enabled.
func Enable(id string) (string, error) { return Apply(OpEnable, id) }

// Remove deletes job's line entirely.
func Remove(id string) (string, error) { return Apply(OpRemove, id) }

// checkTimerOp validates a disable or enable of a systemd timer.
func checkTimerOp(op Operation, t Job) error {
	switch {
	case t.ReadOnly:
		return fmt.Errorf("timer %q cannot be changed here: %s", t.Unit, t.Note)
	case op == OpRemove:
		return fmt.Errorf("Seer does not delete systemd timers; remove %s yourself", t.Source)
	case op == OpDisable && !t.Enabled:
		return fmt.Errorf("job %q is already disabled", t.ID)
	case op == OpEnable && t.Enabled:
		return fmt.Errorf("job %q is already enabled", t.ID)
	}

	return nil
}

// applyTimer enables or disables a timer through systemctl. Nothing is
// backed up: undo is the opposite command.
func applyTimer(op Operation, t Job) error {
	if err := checkTimerOp(op, t); err != nil {
		return err
	}

	return systemctlAction(op, t)
}
