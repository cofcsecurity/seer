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

func jobOutput(j job.Job, detail, color bool) string {
	line := j.String()
	if detail {
		line = j.Describe()
	}
	if !color || j.Enabled {
		return line
	}
	return paint(strings.TrimSuffix(line, "\n"), ansiYellow, true) + "\n"
}
