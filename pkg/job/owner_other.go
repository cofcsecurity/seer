package job

import (
	"os"
	"strconv"
	"syscall"
)

// ownerUID returns the numeric owner of a file as text, or "" if unknown.
func ownerUID(info os.FileInfo) string {
	if stat, ok := info.Sys().(*syscall.Stat_t); ok {
		return strconv.FormatUint(uint64(stat.Uid), 10)
	}

	return ""
}
