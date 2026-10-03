package job

import (
	"io"
	"os"
	"strings"

	"seer/pkg/job"
)

const (
	ansiReset   = "\x1b[0m"
	ansiCyan    = "\x1b[1;36m"
	ansiYellow  = "\x1b[1;33m"
	ansiRed     = "\x1b[1;31m"
	ansiGreen   = "\x1b[1;32m"
	ansiBlue    = "\x1b[1;34m"
	ansiMagenta = "\x1b[1;35m"
	ansiDim     = "\x1b[2m"
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

var roleColors = map[job.Role]string{
	job.RoleID:          ansiDim,
	job.RoleCommand:     "",
	job.RoleDisabled:    ansiYellow,
	job.RoleUserRoot:    ansiRed,
	job.RoleUserSystem:  ansiMagenta,
	job.RoleUserRegular: ansiGreen,
	job.RoleUserUnknown: ansiDim,
	job.RoleRisk:        ansiRed,
	job.RoleReadOnly:    ansiBlue,
	job.RoleReboot:      ansiMagenta,
	job.RoleMacro:       ansiBlue,
	job.RoleFrequent:    ansiYellow,
	job.RoleStandard:    ansiCyan,
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
	if !color || j.Enabled {
		return line
	}
	return paint(strings.TrimSuffix(line, "\n"), ansiDim, true) + "\n"
}

// legend explains the colors and tags used in job output.
func legend(color bool) string {
	var style job.Style
	if color {
		style = roleStyle
	}
	parts := []string{
		"owner: " + style.Apply(job.RoleUserRoot, "root") + " " +
			style.Apply(job.RoleUserSystem, "system") + " " +
			style.Apply(job.RoleUserRegular, "user"),
		"schedule: " + style.Apply(job.RoleReboot, "@reboot") + " " +
			style.Apply(job.RoleMacro, "@macro") + " " +
			style.Apply(job.RoleFrequent, "frequent") + " " +
			style.Apply(job.RoleStandard, "fixed"),
		style.Apply(job.RoleRisk, "[RISK]") + " root job with a tamperable command",
		style.Apply(job.RoleReadOnly, "[periodic]") + " " + style.Apply(job.RoleReadOnly, "[anacron]") + " read-only",
		style.Apply(job.RoleDisabled, "[DISABLED]"),
	}
	return "Legend: " + strings.Join(parts, "; ") + "\n"
}
