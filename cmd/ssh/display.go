package ssh

import (
	"io"
	"os"
	"seer/pkg/ssh"
	"strings"
)

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
		return "\x1b[1;33m" + strings.TrimSuffix(line, "\n") + "\x1b[0m\n"
	}
	return line
}

func warningOutput(message string, color bool) string {
	if color {
		return "\x1b[1;31m" + message + "\x1b[0m"
	}
	return message
}
