package job

import (
	"bytes"
	"encoding/json"
	"fmt"
	"hash/fnv"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"
)

const backupPrefix = ".seer-job-backup-"

// Backup is a copy of a cron file saved before Seer changed it.
type Backup struct {
	ID     string
	Path   string
	Source string // the file it protects; empty for backups in the old naming
	Op     string // disable, enable, remove, or restore
	Time   time.Time
	Size   int64
	Meta   *Meta // which line changed; nil when no sidecar was saved
}

// Meta is saved beside a backup (as "<backup>.meta") and records the single
// line change that produced it, plus the file's mode and owner, so the change
// can be undone precisely and a deleted file can be recreated.
type Meta struct {
	Targeted bool   `json:"targeted"` // the line fields below are valid
	Line     int    `json:"line"`     // 1-based line number before the change
	Before   string `json:"before"`   // the line above it, as an anchor
	HasOld   bool   `json:"has_old"`
	Old      string `json:"old"` // the line before the change
	HasNew   bool   `json:"has_new"`
	New      string `json:"new"`     // the line after the change; absent for remove
	Created  bool   `json:"created"` // the change created the file
	Mode     uint32 `json:"mode"`
	UID      int    `json:"uid"`
	GID      int    `json:"gid"`
}

const metaSuffix = ".meta"

func fileMeta(info os.FileInfo) Meta {
	m := Meta{Mode: uint32(info.Mode().Perm())}
	if stat, ok := info.Sys().(*syscall.Stat_t); ok {
		m.UID, m.GID = int(stat.Uid), int(stat.Gid)
	}

	return m
}

func writeMeta(backupPath string, m Meta) error {
	data, err := json.Marshal(m)
	if err != nil {
		return err
	}

	return os.WriteFile(backupPath+metaSuffix, data, 0o600)
}

func readMeta(backupPath string) *Meta {
	data, err := os.ReadFile(backupPath + metaSuffix)
	if err != nil {
		return nil
	}

	var m Meta
	if json.Unmarshal(data, &m) != nil {
		return nil
	}

	return &m
}

// Summary describes the job line a backup is about, without any marker.
func (b Backup) Summary() string {
	if b.Meta == nil || !b.Meta.Targeted {
		return ""
	}
	if b.Meta.HasOld {
		return idText(b.Meta.Old)
	}

	return idText(b.Meta.New)
}

// BackupID returns the short ID used to refer to the backup at path.
func BackupID(path string) string {
	h := fnv.New64a()
	_, _ = h.Write([]byte(path))

	return fmt.Sprintf("%016x", h.Sum64())
}

// backupDirs lists the directories Seer writes backups into.
func backupDirs() []string {
	dirs := []string{filepath.Dir(crontabPath), cronDDir}
	dirs = append(dirs, spoolRoots...)

	seen := map[string]bool{}
	var out []string
	for _, d := range dirs {
		if !seen[d] {
			seen[d] = true
			out = append(out, d)
		}
	}

	return out
}

// parseBackupName splits ".seer-job-backup-<op>-<file>-<random>".
func parseBackupName(name string) (op, base string, ok bool) {
	rest, found := strings.CutPrefix(name, backupPrefix)
	if !found {
		return "", "", false
	}

	op, rest, found = strings.Cut(rest, "-")
	if !found {
		return "", "", false
	}

	i := strings.LastIndex(rest, "-")
	if i <= 0 {
		return "", "", false
	}
	if _, err := strconv.Atoi(rest[i+1:]); err != nil {
		return "", "", false
	}

	return op, rest[:i], true
}

// Backups returns every backup Seer has saved, newest first.
func Backups() (backups []Backup, warnings []string) {
	for _, dir := range backupDirs() {
		entries, err := os.ReadDir(dir)
		if err != nil {
			if !os.IsNotExist(err) {
				warnings = append(warnings, fmt.Sprintf("could not read %s: %v", dir, err))
			}
			continue
		}

		for _, entry := range entries {
			if !strings.HasPrefix(entry.Name(), backupPrefix) || strings.HasSuffix(entry.Name(), metaSuffix) {
				continue
			}

			info, err := entry.Info()
			if err != nil || !info.Mode().IsRegular() {
				continue
			}

			path := filepath.Join(dir, entry.Name())
			b := Backup{ID: BackupID(path), Path: path, Time: info.ModTime(), Size: info.Size(), Meta: readMeta(path)}
			if op, base, ok := parseBackupName(entry.Name()); ok {
				b.Op, b.Source = op, filepath.Join(dir, base)
			}
			backups = append(backups, b)
		}
	}

	sort.Slice(backups, func(i, j int) bool { return backups[i].Time.After(backups[j].Time) })

	return backups, warnings
}

func findBackup(id string) (Backup, error) {
	backups, _ := Backups()
	for _, b := range backups {
		if b.ID == id {
			return b, nil
		}
	}

	return Backup{}, fmt.Errorf("backup %q not found; run 'seer job backups' for the list", id)
}

// restoreOp is a planned restore: what the file holds now and what it would
// hold afterwards.
type restoreOp struct {
	backup   Backup
	current  []byte // nil when the source file no longer exists
	content  []byte // what the source would contain afterwards
	info     os.FileInfo
	undoMeta Meta
}

// restorePlan works out the file contents a restore would write.
//
// By default a backup that recorded its line change is undone surgically:
// only that line is put back (or reverted), leaving any other edits made
// since alone. wholeFile, or a backup without a record, replaces the whole
// file with the saved copy instead.
func restorePlan(id string, wholeFile bool) (p restoreOp, err error) {
	p.backup, err = findBackup(id)
	if err != nil {
		return p, err
	}
	b := p.backup

	if b.Source == "" {
		return p, fmt.Errorf("backup %q uses an old naming scheme and does not record its source file; copy %s back by hand", id, b.Path)
	}

	saved, err := os.ReadFile(b.Path)
	if err != nil {
		return p, err
	}

	p.info, err = os.Lstat(b.Source)
	switch {
	case err == nil && p.info.Mode().IsRegular():
		if p.current, err = os.ReadFile(b.Source); err != nil {
			return p, err
		}
	case os.IsNotExist(err):
		// The file is gone: recreate it from the backup.
		if b.Meta == nil {
			return p, fmt.Errorf("%s no longer exists and the backup has no record of its mode and owner; copy %s back by hand", b.Source, b.Path)
		}
		p.info = nil
		p.content = saved
		p.undoMeta = Meta{}
		return p, nil
	default:
		return p, fmt.Errorf("%s is not a regular file", b.Source)
	}

	p.undoMeta = fileMeta(p.info)

	if wholeFile || b.Meta == nil || !b.Meta.Targeted {
		p.content = saved
	} else {
		lines := strings.SplitAfter(string(p.current), "\n")
		if lines[len(lines)-1] == "" {
			lines = lines[:len(lines)-1]
		}

		undone, err := undoChange(lines, *b.Meta)
		if err != nil {
			return p, fmt.Errorf("%w; use --whole-file to replace the whole file with the backup instead", err)
		}

		p.content = []byte(strings.Join(undone, ""))

		// Undoing this restore means reversing the same line change.
		p.undoMeta.Targeted, p.undoMeta.Line, p.undoMeta.Before = true, b.Meta.Line, b.Meta.Before
		p.undoMeta.HasOld, p.undoMeta.Old = b.Meta.HasNew, b.Meta.New
		p.undoMeta.HasNew, p.undoMeta.New = b.Meta.HasOld, b.Meta.Old
	}

	if bytes.Equal(p.content, p.current) {
		return p, fmt.Errorf("%s already matches the backup", b.Source)
	}

	return p, nil
}

func trimEOL(line string) string { return strings.TrimRight(line, "\r\n") }

// undoChange reverses one recorded line change on lines (which keep their
// line endings).
func undoChange(lines []string, m Meta) ([]string, error) {
	if m.HasNew {
		// Find the changed line, preferring the one nearest where it was.
		at := -1
		for i, l := range lines {
			if trimEOL(l) == m.New && (at < 0 || abs(i-(m.Line-1)) < abs(at-(m.Line-1))) {
				at = i
			}
		}
		if at < 0 {
			return nil, fmt.Errorf("the line %q is no longer in the file, so it has changed since the backup", m.New)
		}

		out := append([]string(nil), lines...)
		if m.HasOld {
			out[at] = m.Old + lineEnding(lines[at])
		} else {
			out = append(out[:at], out[at+1:]...)
		}

		return out, nil
	}

	// The line was removed: put it back.
	for _, l := range lines {
		if trimEOL(l) == m.Old {
			return nil, fmt.Errorf("the line %q is already in the file", m.Old)
		}
	}

	at := min(max(m.Line-1, 0), len(lines))
	if m.Before != "" && (at == 0 || trimEOL(lines[at-1]) != m.Before) {
		for i, l := range lines {
			if trimEOL(l) == m.Before {
				at = i + 1
				break
			}
		}
	}

	out := append([]string(nil), lines[:at]...)
	if at > 0 && !strings.HasSuffix(out[at-1], "\n") {
		out[at-1] += "\n"
	}
	out = append(out, m.Old+"\n")

	return append(out, lines[at:]...), nil
}

func abs(n int) int {
	if n < 0 {
		return -n
	}

	return n
}

// RestoreDiff shows how restoring the backup would change its source file,
// without writing anything.
func RestoreDiff(id string, wholeFile bool) (string, error) {
	p, err := restorePlan(id, wholeFile)
	if err != nil {
		return "", err
	}

	header := fmt.Sprintf("--- %s (now)\n+++ %s (after restore)\n", p.backup.Source, p.backup.Source)
	if p.current == nil {
		header = fmt.Sprintf("%s does not exist; it would be recreated:\n", p.backup.Source)
	}

	return header + lineDiff(splitLines(p.current), splitLines(p.content)), nil
}

// RestoreSummary describes a line-level restore in a few words. It returns
// false when the restore replaces the whole file, where RestoreDiff is the
// better description.
func RestoreSummary(id string, wholeFile bool) (string, bool, error) {
	p, err := restorePlan(id, wholeFile)
	if err != nil {
		return "", false, err
	}

	m := p.backup.Meta
	if p.current == nil || wholeFile || m == nil || !m.Targeted {
		return "", false, nil
	}

	src := p.backup.Source
	switch {
	case m.HasNew && m.HasOld:
		return fmt.Sprintf("Change in %s:\n  - %s\n  + %s\n", src, m.New, m.Old), true, nil
	case m.HasNew:
		return fmt.Sprintf("Remove from %s:\n  - %s\n", src, m.New), true, nil
	default:
		return fmt.Sprintf("Re-add to %s:\n  + %s\n", src, m.Old), true, nil
	}
}

// Restore undoes the change the backup records, or with wholeFile puts the
// whole saved copy back. The current contents are saved first as a new
// backup, so a restore can itself be undone. It returns that backup's path
// (empty when the file had to be recreated).
func Restore(id string, wholeFile bool) (undoPath string, err error) {
	p, err := restorePlan(id, wholeFile)
	if err != nil {
		return "", err
	}

	source := p.backup.Source
	dir := filepath.Dir(source)

	if p.current == nil {
		return "", recreate(source, p.content, *p.backup.Meta)
	}

	undoPath, err = writeBackup(dir, filepath.Base(source), string(OpRestore), p.current)
	if err != nil {
		return "", fmt.Errorf("could not create backup: %w", err)
	}
	_ = writeMeta(undoPath, p.undoMeta)

	tmpPath, err := writeReplacement(dir, source, p.info, p.content)
	if err != nil {
		return undoPath, fmt.Errorf("could not stage replacement: %w", err)
	}

	if err := commitReplacement(source, p.info, p.current, tmpPath); err != nil {
		_ = os.Remove(tmpPath)
		return undoPath, err
	}

	// Undoing an add that created the file leaves nothing behind.
	if p.backup.Meta != nil && p.backup.Meta.Created && strings.TrimSpace(string(p.content)) == "" {
		_ = os.Remove(source)
	}

	return undoPath, nil
}

// recreate writes a deleted source file back with its saved mode and owner.
func recreate(path string, data []byte, m Meta) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), ".seer-job-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())

	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Chmod(os.FileMode(m.Mode)); err != nil {
		tmp.Close()
		return err
	}
	if m.UID != os.Geteuid() || m.GID != os.Getegid() {
		if err := tmp.Chown(m.UID, m.GID); err != nil {
			tmp.Close()
			return err
		}
	}
	if err := tmp.Close(); err != nil {
		return err
	}

	return os.Rename(tmp.Name(), path)
}

// SelectBackups returns the backups older than age (all of them when age is
// zero or negative) as of now.
func SelectBackups(backups []Backup, age time.Duration, now time.Time) []Backup {
	var out []Backup
	for _, b := range backups {
		if age <= 0 || now.Sub(b.Time) >= age {
			out = append(out, b)
		}
	}

	return out
}

// RemoveBackups deletes the given backups and returns how many were removed.
func RemoveBackups(backups []Backup) (removed int, err error) {
	for _, b := range backups {
		if err := os.Remove(b.Path); err != nil && !os.IsNotExist(err) {
			return removed, err
		}
		_ = os.Remove(b.Path + metaSuffix)
		removed++
	}

	return removed, nil
}

// ParseAge accepts Go durations plus d (days) and w (weeks), such as 30d.
func ParseAge(text string) (time.Duration, error) {
	for suffix, unit := range map[string]time.Duration{"d": 24 * time.Hour, "w": 7 * 24 * time.Hour} {
		if num, ok := strings.CutSuffix(text, suffix); ok {
			n, err := strconv.ParseFloat(num, 64)
			if err != nil || n < 0 {
				return 0, fmt.Errorf("invalid age %q", text)
			}
			return time.Duration(n * float64(unit)), nil
		}
	}

	d, err := time.ParseDuration(text)
	if err != nil || d < 0 {
		return 0, fmt.Errorf("invalid age %q (try 12h, 30d, or 2w)", text)
	}

	return d, nil
}

func splitLines(data []byte) []string {
	text := strings.TrimSuffix(string(data), "\n")
	if text == "" {
		return nil
	}

	return strings.Split(text, "\n")
}

// lineDiff returns only the changed lines of a longest-common-subsequence
// diff, each prefixed with "-" (only in a) or "+" (only in b).
func lineDiff(a, b []string) string {
	lcs := make([][]int, len(a)+1)
	for i := range lcs {
		lcs[i] = make([]int, len(b)+1)
	}
	for i := len(a) - 1; i >= 0; i-- {
		for j := len(b) - 1; j >= 0; j-- {
			if a[i] == b[j] {
				lcs[i][j] = lcs[i+1][j+1] + 1
			} else {
				lcs[i][j] = max(lcs[i+1][j], lcs[i][j+1])
			}
		}
	}

	var out strings.Builder
	i, j := 0, 0
	for i < len(a) || j < len(b) {
		switch {
		case i < len(a) && j < len(b) && a[i] == b[j]:
			i++
			j++
		case j < len(b) && (i == len(a) || lcs[i][j+1] >= lcs[i+1][j]):
			fmt.Fprintf(&out, "+%s\n", b[j])
			j++
		default:
			fmt.Fprintf(&out, "-%s\n", a[i])
			i++
		}
	}

	return out.String()
}
