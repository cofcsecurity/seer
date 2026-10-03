package job

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
)

var tempPrefixes = []string{"/tmp/", "/var/tmp/", "/dev/shm/"}

// jobRisks reports ways a non-root user could alter what an enabled root job
// runs: the command file or one of its parent directories being writable by
// others or owned by someone other than root, or living in a temp directory.
// Only an absolute path as the first command word is checked; commands found
// through PATH are not.
func jobRisks(j Job) []string {
	if !j.Enabled || j.UserRole() != RoleUserRoot {
		return nil
	}

	path := commandPath(j.Command)
	if path == "" {
		return nil
	}

	var risks []string

	for _, prefix := range tempPrefixes {
		if strings.HasPrefix(path, prefix) {
			risks = append(risks, "runs from temporary directory "+strings.TrimSuffix(prefix, "/"))
		}
	}

	seen := map[string]bool{}
	add := func(reason string) {
		if !seen[reason] {
			seen[reason] = true
			risks = append(risks, reason)
		}
	}

	for _, p := range pathsToCheck(path) {
		if reason := weakPermissions(p); reason != "" {
			add(reason)
		}
	}

	return risks
}

// commandPath returns the first word of command when it is an absolute path,
// skipping leading VAR=value assignments.
func commandPath(command string) string {
	for _, word := range strings.Fields(command) {
		if strings.Contains(word, "=") && !strings.HasPrefix(word, "/") {
			continue
		}
		if strings.HasPrefix(word, "/") {
			return filepath.Clean(strings.Trim(word, `"'`))
		}
		return ""
	}

	return ""
}

// pathsToCheck returns the path, its symlink target if different, and each
// parent directory of both.
func pathsToCheck(path string) []string {
	targets := []string{path}
	if resolved, err := filepath.EvalSymlinks(path); err == nil && resolved != path {
		targets = append(targets, resolved)
	}

	var out []string
	for _, t := range targets {
		for p := t; ; p = filepath.Dir(p) {
			out = append(out, p)
			if p == "/" || p == "." {
				break
			}
		}
	}

	return out
}

func weakPermissions(path string) string {
	info, err := os.Lstat(path)
	if err != nil {
		return ""
	}

	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return ""
	}

	if info.Mode()&os.ModeSymlink != 0 {
		// A symlink's own mode is irrelevant; its directory and target
		// are checked separately.
		return ""
	}

	switch {
	case stat.Uid != 0:
		return fmt.Sprintf("%s is owned by uid %d, not root", path, stat.Uid)
	case info.Mode().Perm()&0o002 != 0 && !(info.IsDir() && info.Mode()&os.ModeSticky != 0):
		return path + " is writable by any user"
	case info.Mode().Perm()&0o020 != 0 && stat.Gid != 0:
		return fmt.Sprintf("%s is writable by group %d", path, stat.Gid)
	}

	return ""
}
