package warden

import (
	"errors"
	"io"
	"os"
	"runtime"

	"github.com/spf13/cobra"
)

func Warden() *cobra.Command {
	warden := &cobra.Command{
		Use:   "warden",
		Short: "Install, find, and check Warden or Quiet Warden on this box",
		Long: "Obtain and install Warden (cofcsecurity/warden) or Quiet Warden (cofcsecurity/quiet-warden),\n" +
			"then find the install and report its health. Both repositories are private: release\n" +
			"downloads and clones read a GitHub token from GITHUB_TOKEN, GH_TOKEN, or 'gh auth token'.",
	}
	warden.AddCommand(Install())
	warden.AddCommand(Find())
	warden.AddCommand(Status())
	warden.AddCommand(Uninstall())
	return warden
}

const (
	ansiReset  = "\x1b[0m"
	ansiCyan   = "\x1b[1;36m"
	ansiYellow = "\x1b[1;33m"
	ansiRed    = "\x1b[1;31m"
	ansiGreen  = "\x1b[1;32m"
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

// requireRoot fails early with a clear message; Warden itself needs root
// and Linux for every command Seer runs.
func requireRoot() error {
	if runtime.GOOS != "linux" {
		return errors.New("Warden runs on Linux only")
	}
	if os.Geteuid() != 0 {
		return errors.New("run as root (sudo seer warden ...)")
	}
	return nil
}
