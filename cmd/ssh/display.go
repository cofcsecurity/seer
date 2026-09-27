package ssh

import (
	"fmt"
	"io"
	"os"
	"seer/pkg/ssh"
	"strings"
)

const (
	ansiReset  = "\x1b[0m"
	ansiDim    = "\x1b[2m"
	ansiCyan   = "\x1b[1;36m"
	ansiGreen  = "\x1b[1;32m"
	ansiYellow = "\x1b[1;33m"
	ansiRed    = "\x1b[1;31m"
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

func sessionOutput(s ssh.Session, detail, color bool) string {
	line := s.String()
	if detail {
		line = s.Describe()
	}
	if s.Current && color {
		return paint(strings.TrimSuffix(line, "\n"), ansiYellow, true) + "\n"
	}
	if !color {
		return line
	}
	if detail {
		return labelOutput(line, true)
	}
	if id, rest, ok := strings.Cut(line, "]"); ok {
		if s.Authenticated {
			rest = strings.Replace(rest, " authenticated", " "+paint("authenticated", ansiGreen, true), 1)
		} else {
			rest = strings.Replace(rest, " auth unknown", " "+paint("auth unknown", ansiYellow, true), 1)
		}
		return paint(id+"]", ansiCyan, true) + rest
	}
	return line
}

func warningOutput(message string, color bool) string {
	return paint(message, ansiRed, color)
}

// labelOutput highlights field names while preserving the exact plain text.
func labelOutput(value string, color bool) string {
	if !color {
		return value
	}
	lines := strings.SplitAfter(value, "\n")
	for i, line := range lines {
		if strings.HasPrefix(line, "┌ ") {
			lines[i] = paint(strings.TrimSuffix(line, "\n"), ansiCyan, true)
			if strings.HasSuffix(line, "\n") {
				lines[i] += "\n"
			}
			continue
		}
		if at := strings.Index(line, ":"); at >= 0 {
			lines[i] = paint(line[:at+1], ansiCyan, true) + line[at+1:]
		}
	}
	return strings.Join(lines, "")
}

func configRuleOutput(rule ssh.ConfigRule, color bool) string {
	location := fmt.Sprintf("%s:%d", rule.Path, rule.Line)
	style := ansiCyan
	if strings.EqualFold(rule.Key, "Match") {
		style = ansiYellow
	}
	return fmt.Sprintf("%s %s %s\n", paint(location, ansiDim, color), paint(rule.Key, style, color), rule.Value)
}

func configFindingOutput(finding ssh.ConfigFinding, color bool) string {
	style := ansiYellow
	if finding.Level == "warning" {
		style = ansiRed
	}
	return fmt.Sprintf("%s: %s=%s: %s\n",
		paint(finding.Level, style, color), paint(finding.Key, ansiCyan, color),
		paint(finding.Value, style, color), finding.Reason)
}

func configEffectiveOutput(key, value, level string, color bool) string {
	style := ""
	switch level {
	case "warning":
		style = ansiRed
	case "review":
		style = ansiYellow
	}
	if style != "" {
		value = paint(value, style, color)
	}
	return fmt.Sprintf("%s %s\n", paint(key, ansiCyan, color), value)
}

func keyListOutput(key ssh.AuthorizedKey, color bool) string {
	return fmt.Sprintf("%s %s %s %s %s\n",
		key.User, paint("SHA256:"+key.Fingerprint, ansiCyan, color), key.Type,
		paint(fmt.Sprintf("%s:%d", key.Path, key.Line), ansiDim, color), key.Comment)
}

func sourcesOutput(value string, color bool) string {
	if !color {
		return value
	}
	lines := strings.SplitAfter(value, "\n")
	for i, line := range lines {
		if strings.HasPrefix(line, "Authentication sources for ") {
			lines[i] = paint(strings.TrimSuffix(line, "\n"), ansiCyan, true)
			if strings.HasSuffix(line, "\n") {
				lines[i] += "\n"
			}
		} else if strings.HasPrefix(line, "Warning:") {
			lines[i] = paint(strings.TrimSuffix(line, "\n"), ansiRed, true)
			if strings.HasSuffix(line, "\n") {
				lines[i] += "\n"
			}
		} else if at := strings.Index(line, ":"); at >= 0 {
			lines[i] = paint(line[:at+1], ansiCyan, true) + line[at+1:]
		}
	}
	return strings.Join(lines, "")
}

func historyOutput(value string, failed, color bool) string {
	if !color {
		return value
	}
	lines := strings.SplitAfter(value, "\n")
	for i, line := range lines {
		plain := strings.TrimSuffix(line, "\n")
		if plain == "" {
			continue
		}
		style := ""
		lower := strings.ToLower(plain)
		switch {
		case strings.HasPrefix(plain, "SSH daemon ") || strings.HasPrefix(plain, "System login records"):
			style = ansiCyan
		case failed || strings.Contains(lower, "failed ") || strings.Contains(lower, "invalid user ") ||
			strings.Contains(lower, "authentication failure"):
			style = ansiRed
		case strings.Contains(lower, "accepted "):
			style = ansiGreen
		}
		if style != "" {
			lines[i] = paint(plain, style, true)
			if strings.HasSuffix(line, "\n") {
				lines[i] += "\n"
			}
		}
	}
	return strings.Join(lines, "")
}
