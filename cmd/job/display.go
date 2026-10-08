package job

import (
	"io"
	"os"
	"strings"

	"seer/pkg/job"
)

const (
	ansiReset  = "\x1b[0m"
	ansiCyan   = "\x1b[1;36m"
	ansiYellow = "\x1b[1;33m"
	ansiRed    = "\x1b[1;31m"
	ansiGreen  = "\x1b[1;32m"
	ansiBold   = "\x1b[1m"
)

func paint(value, style string, color bool) string {
	if !color {
		return value
	}
	return style + value + ansiReset
}

func terminalColor(out io.Writer) bool {
	if _, disabled := os.LookupEnv("NO_COLOR"); disabled || os.Getenv("TERM") == "" || os.Getenv("TERM") == "dumb" {
		return false
	}
	file, ok := out.(*os.File)
	if !ok {
		return false
	}
	info, err := file.Stat()
	return err == nil && info.Mode()&os.ModeCharDevice != 0
}

// A deliberately small palette: red for risk, yellow for things that need a
// look, cyan for non-root owners, and bold for root. Nothing is dimmed, since
// dim text has too little contrast for many readers; every marker is also a
// text label, so no meaning depends on color alone.
var roleColors = map[job.Role]string{
	job.RoleDisabled:    ansiYellow,
	job.RoleFrequent:    ansiYellow,
	job.RoleRisk:        ansiRed,
	job.RoleUserRoot:    ansiBold,
	job.RoleUserRegular: ansiCyan,
}

func roleStyle(role job.Role, text string) string {
	if style := roleColors[role]; style != "" {
		return style + text + ansiReset
	}
	return text
}

func jobOutput(j job.Job, detail, color bool) string {
	var style job.Style
	if color {
		style = roleStyle
		if len(j.Risks) > 0 {
			style = nil
		}
	}
	line := j.StringStyled(style)
	if detail {
		line = j.DescribeStyled(style)
	}
	if color && len(j.Risks) > 0 {
		// Risky jobs are red from end to end so they stand out.
		lines := strings.Split(strings.TrimSuffix(line, "\n"), "\n")
		for i, l := range lines {
			lines[i] = paint(l, ansiRed, true)
		}
		return strings.Join(lines, "\n") + "\n"
	}
	return line
}

// legend returns a short color key, followed by a blank line, to print above
// job output.
func legend(color bool) string {
	var style job.Style
	if color {
		style = roleStyle
	}
	a := style.Apply
	return "Owner:  " + a(job.RoleUserRoot, "root") + "  " + a(job.RoleUserRegular, "user") + "  system\n" +
		"Marks:  " + a(job.RoleFrequent, "frequent") + "  " + a(job.RoleDisabled, "[DISABLED] [DENIED]") + "  " +
		a(job.RoleFrequent, "[RECENT]") + " changed lately  " + a(job.RoleRisk, "[RISK]") + " tamperable\n" +
		"Types:  [timer] systemd  [at] one-time  [periodic] [anacron] read-only\n\n"
}

// colorDiff colors removed lines red and added lines green.
func colorDiff(text string, color bool) string {
	if !color {
		return text
	}
	lines := strings.Split(strings.TrimSuffix(text, "\n"), "\n")
	for i, l := range lines {
		switch {
		case strings.HasPrefix(l, "---"), strings.HasPrefix(l, "+++"), strings.HasPrefix(l, "@@"):
			lines[i] = paint(l, ansiCyan, true)
		case strings.HasPrefix(l, "-"):
			lines[i] = paint(l, ansiRed, true)
		case strings.HasPrefix(l, "+"):
			lines[i] = paint(l, ansiGreen, true)
		}
	}
	return strings.Join(lines, "\n") + "\n"
}

// undoHint returns the command that reverses a change. Cron edits are
// undone from their backup; systemd timers by the opposite verb.
func undoHint(t job.Job, inverse, backup string) string {
	if t.Kind == job.KindTimer {
		return "seer job " + inverse + " " + t.ID
	}
	return "seer job restore " + job.BackupID(backup)
}
