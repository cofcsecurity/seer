package job

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Unit locations, highest precedence first. A variable so tests can use
// temp dirs.
var systemdDirs = []string{
	"/etc/systemd/system",
	"/run/systemd/system",
	"/usr/lib/systemd/system",
	"/lib/systemd/system",
}

// runCommand runs a program and returns its combined output. Tests replace it.
var runCommand = func(name string, args ...string) ([]byte, error) {
	return exec.Command(name, args...).CombinedOutput()
}

// unitFile is a parsed systemd unit: section name to key/value pairs, in order.
type unitFile map[string][][2]string

func (u unitFile) get(section, key string) string {
	for _, kv := range u[section] {
		if kv[0] == key {
			return kv[1]
		}
	}

	return ""
}

func (u unitFile) all(section, key string) []string {
	var out []string
	for _, kv := range u[section] {
		if kv[0] == key && kv[1] != "" {
			out = append(out, kv[1])
		}
	}

	return out
}

// parseUnit reads a systemd unit file. It handles sections, comments and
// backslash line continuations, which is all the timer and service keys need.
func parseUnit(path string) (unitFile, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	unit := unitFile{}
	section := ""
	var pending string

	for _, raw := range strings.Split(string(data), "\n") {
		line := strings.TrimSpace(raw)
		if pending != "" {
			line = pending + " " + line
			pending = ""
		}
		if strings.HasSuffix(line, "\\") {
			pending = strings.TrimSpace(strings.TrimSuffix(line, "\\"))
			continue
		}
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, ";") {
			continue
		}
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			section = line[1 : len(line)-1]
			if _, ok := unit[section]; !ok {
				unit[section] = nil
			}
			continue
		}
		if key, value, ok := strings.Cut(line, "="); ok {
			unit[section] = append(unit[section], [2]string{strings.TrimSpace(key), strings.TrimSpace(value)})
		}
	}

	return unit, nil
}

// findUnit returns the path of the named unit from the highest-precedence
// directory that has it.
func findUnit(name string) string {
	for _, dir := range systemdDirs {
		path := filepath.Join(dir, name)
		if info, err := os.Lstat(path); err == nil && !info.IsDir() {
			return path
		}
	}

	return ""
}

// timerTriggers are the [Timer] keys that decide when a timer fires.
var timerTriggers = []string{
	"OnCalendar", "OnBootSec", "OnStartupSec", "OnActiveSec", "OnUnitActiveSec", "OnUnitInactiveSec",
}

// timerJobs lists systemd timers. A timer is enabled when a .wants symlink
// links it into a target; one with no [Install] section is static and only
// starts through another unit.
func timerJobs() (jobs []Job, warnings []string) {
	seen := map[string]bool{}

	for _, dir := range systemdDirs {
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}

		for _, entry := range entries {
			name := entry.Name()
			if !strings.HasSuffix(name, ".timer") || seen[name] {
				continue
			}
			seen[name] = true

			path := filepath.Join(dir, name)
			// A masked timer is a link to /dev/null.
			if isMasked(path) {
				jobs = append(jobs, maskedTimer(name, path))
				continue
			}
			if info, err := os.Stat(path); err != nil || info.IsDir() {
				continue
			}

			unit, err := parseUnit(path)
			if err != nil {
				warnings = append(warnings, fmt.Sprintf("could not read %s: %v", path, err))
				continue
			}

			jobs = append(jobs, timerJob(name, path, unit))
		}
	}

	sort.Slice(jobs, func(i, j int) bool { return jobs[i].Unit < jobs[j].Unit })

	return jobs, warnings
}

func isMasked(path string) bool {
	target, err := os.Readlink(path)

	return err == nil && target == "/dev/null"
}

func maskedTimer(name, path string) Job {
	return Job{
		ID: jobID(path, name), Source: path, User: "root", Schedule: "masked", Command: name,
		Raw: path, Kind: KindTimer, ReadOnly: true, Unit: name,
		Note: "masked (linked to /dev/null); run 'systemctl unmask " + name + "' to use it",
	}
}

func timerJob(name, path string, unit unitFile) Job {
	var triggers []string
	for _, key := range timerTriggers {
		for _, value := range unit.all("Timer", key) {
			triggers = append(triggers, key+"="+value)
		}
	}
	schedule := strings.Join(triggers, "; ")
	if schedule == "" {
		schedule = "(no trigger)"
	}

	serviceName := unit.get("Timer", "Unit")
	if serviceName == "" {
		serviceName = strings.TrimSuffix(name, ".timer") + ".service"
	}

	j := Job{
		ID: jobID(path, name), Source: path, User: "root", Schedule: schedule,
		Command: serviceName, Raw: path, Kind: KindTimer, Unit: name, Service: serviceName, Enabled: true,
		Modified: modTime(path),
	}

	if servicePath := findUnit(serviceName); servicePath != "" {
		if service, err := parseUnit(servicePath); err == nil {
			if cmd := cleanExec(service.get("Service", "ExecStart")); cmd != "" {
				j.Command = cmd
			}
			if u := service.get("Service", "User"); u != "" {
				j.User = u
			}
			if m := modTime(servicePath); m.After(j.Modified) {
				j.Modified = m
			}
		}
	}

	if _, hasInstall := unit["Install"]; !hasInstall {
		j.ReadOnly = true
		j.Note = "static timer (no [Install] section); it is started by another unit and cannot be enabled or disabled"
		return j
	}

	j.Enabled = timerEnabled(name)

	return j
}

func modTime(path string) time.Time {
	if info, err := os.Stat(path); err == nil {
		return info.ModTime()
	}

	return time.Time{}
}

// cleanExec drops systemd's ExecStart prefixes (-, @, +, !, :) from a command.
func cleanExec(cmd string) string {
	return strings.TrimSpace(strings.TrimLeft(cmd, "-@+!: \t"))
}

// timerEnabled reports whether a .wants directory links the timer in.
func timerEnabled(name string) bool {
	for _, dir := range systemdDirs {
		matches, _ := filepath.Glob(filepath.Join(dir, "*.wants", name))
		if len(matches) > 0 {
			return true
		}
	}

	return false
}

// systemctlAction runs enable/disable on a timer. Both start or stop it too,
// so "disable" really silences the job.
func systemctlAction(op Operation, j Job) error {
	verb := "disable"
	if op == OpEnable {
		verb = "enable"
	}

	out, err := runCommand("systemctl", verb, "--now", j.Unit)
	if err != nil {
		return fmt.Errorf("systemctl %s --now %s failed: %w: %s", verb, j.Unit, err, strings.TrimSpace(string(out)))
	}

	return nil
}

// TimerStatus asks systemd for a timer's live state. It returns nil when
// systemctl is unavailable or the timer is unknown.
func TimerStatus(j Job) map[string]string {
	out, err := runCommand("systemctl", "show", j.Unit,
		"-p", "ActiveState", "-p", "NextElapseUSecRealtime", "-p", "LastTriggerUSec")
	if err != nil {
		return nil
	}

	status := map[string]string{}
	for _, line := range strings.Split(string(out), "\n") {
		if key, value, ok := strings.Cut(line, "="); ok && value != "" && value != "n/a" {
			status[key] = value
		}
	}

	return status
}

// Shortest timer interval, used to color very frequent timers.
var spanPattern = regexp.MustCompile(`^(\d+(?:\.\d+)?)\s*(us|ms|s|sec|second|seconds|m|min|minute|minutes|h|hr|hour|hours)?$`)

// timerFrequent reports whether a timer schedule fires every few minutes.
func timerFrequent(schedule string) bool {
	for _, part := range strings.Split(schedule, "; ") {
		key, value, _ := strings.Cut(part, "=")
		switch key {
		case "OnCalendar":
			switch strings.TrimSpace(value) {
			case "minutely", "*-*-* *:*:00", "*:0/1", "*:*":
				return true
			}
		case "OnUnitActiveSec", "OnUnitInactiveSec", "OnActiveSec":
			if m := spanPattern.FindStringSubmatch(strings.TrimSpace(value)); m != nil {
				n, _ := strconv.ParseFloat(m[1], 64)
				unit := map[string]float64{"": 1, "us": 1e-6, "ms": 1e-3, "s": 1, "sec": 1, "second": 1, "seconds": 1,
					"m": 60, "min": 60, "minute": 60, "minutes": 60, "h": 3600, "hr": 3600, "hour": 3600, "hours": 3600}[m[2]]
				if n*unit <= 300 {
					return true
				}
			}
		}
	}

	return false
}
