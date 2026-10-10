package job

import (
	"bufio"
	"os"
	"os/user"
	"sort"
	"strconv"
	"strings"
	"syscall"
)

// Finding is one result of the overlap check.
type Finding struct {
	Kind    string   // duplicate, similar, or shared-write
	Message string   // what was found
	Jobs    []string // IDs of the jobs involved
}

// Account databases, variables so tests can use fixtures.
var (
	groupFile  = "/etc/group"
	passwdFile = "/etc/passwd"
)

// Check looks for overlaps among enabled jobs:
//   - duplicate: same owner, schedule, and command more than once
//   - similar: same command run under different schedules or owners
//   - shared-write: a job's command lives somewhere another account that has
//     its own jobs can write, so that account can change what the job runs
func Check(jobs []Job) []Finding {
	var enabled []Job
	for _, j := range jobs {
		if j.Enabled && j.Command != "" {
			enabled = append(enabled, j)
		}
	}

	var findings []Finding
	findings = append(findings, duplicateFindings(enabled)...)
	findings = append(findings, shared(enabled)...)

	return findings
}

func normalize(command string) string { return strings.Join(strings.Fields(command), " ") }

func ids(jobs []Job) []string {
	out := make([]string, len(jobs))
	for i, j := range jobs {
		out[i] = j.ID
	}

	return out
}

func duplicateFindings(jobs []Job) []Finding {
	exact := map[string][]Job{}
	byCommand := map[string][]Job{}
	var exactKeys, commandKeys []string

	for _, j := range jobs {
		cmd := normalize(j.Command)
		key := j.User + "\x00" + j.Schedule + "\x00" + cmd
		if _, ok := exact[key]; !ok {
			exactKeys = append(exactKeys, key)
		}
		exact[key] = append(exact[key], j)

		if _, ok := byCommand[cmd]; !ok {
			commandKeys = append(commandKeys, cmd)
		}
		byCommand[cmd] = append(byCommand[cmd], j)
	}

	var out []Finding
	for _, key := range exactKeys {
		if group := exact[key]; len(group) > 1 {
			out = append(out, Finding{
				Kind:    "duplicate",
				Message: strconv.Itoa(len(group)) + " identical jobs: " + group[0].Schedule + " " + group[0].User + " " + strconv.Quote(group[0].Command),
				Jobs:    ids(group),
			})
		}
	}

	for _, cmd := range commandKeys {
		group := byCommand[cmd]
		variants := map[string]bool{}
		for _, j := range group {
			variants[j.User+"\x00"+j.Schedule] = true
		}
		if len(variants) > 1 {
			out = append(out, Finding{
				Kind:    "similar",
				Message: "same command with different schedules or owners: " + strconv.Quote(group[0].Command),
				Jobs:    ids(group),
			})
		}
	}

	return out
}

// shared reports commands whose file or directories can be changed by an
// account other than the job's owner that has jobs of its own.
func shared(jobs []Job) []Finding {
	owned := map[string][]Job{}
	for _, j := range jobs {
		owned[j.User] = append(owned[j.User], j)
	}

	var out []Finding
	for _, j := range jobs {
		path := commandPath(j.Command)
		if path == "" {
			continue
		}

		reported := map[string]bool{}
		for _, p := range pathsToCheck(path) {
			for _, writer := range pathWriters(p) {
				if writer == j.User || len(owned[writer]) == 0 || reported[writer] {
					continue
				}
				reported[writer] = true

				others := ids(owned[writer])
				out = append(out, Finding{
					Kind: "shared-write",
					Message: j.User + " runs " + path + ", but " + writer + " can change " + p +
						" and has " + strconv.Itoa(len(others)) + " job(s) of its own",
					Jobs: append([]string{j.ID}, others...),
				})
			}
		}
	}

	return out
}

// pathWriters lists the accounts other than root that can modify path: its
// owner, and members of its group when it is group-writable.
func pathWriters(path string) []string {
	info, err := os.Lstat(path)
	if err != nil || info.Mode()&os.ModeSymlink != 0 {
		return nil
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return nil
	}

	var writers []string
	if stat.Uid != 0 {
		if u, err := user.LookupId(strconv.FormatUint(uint64(stat.Uid), 10)); err == nil {
			writers = append(writers, u.Username)
		}
	}
	if info.Mode().Perm()&0o020 != 0 && stat.Gid != 0 {
		writers = append(writers, groupMembers(stat.Gid)...)
	}

	sort.Strings(writers)

	return writers
}

// groupMembers returns the accounts in a group: those listed in the group
// file and those whose primary group it is.
func groupMembers(gid uint32) []string {
	want := strconv.FormatUint(uint64(gid), 10)
	seen := map[string]bool{}

	scanFile(groupFile, func(f []string) {
		if len(f) >= 4 && f[2] == want {
			for _, m := range strings.Split(f[3], ",") {
				if m != "" {
					seen[m] = true
				}
			}
		}
	})
	scanFile(passwdFile, func(f []string) {
		if len(f) >= 4 && f[3] == want {
			seen[f[0]] = true
		}
	})

	out := make([]string, 0, len(seen))
	for name := range seen {
		out = append(out, name)
	}
	sort.Strings(out)

	return out
}

func scanFile(path string, fn func(fields []string)) {
	file, err := os.Open(path)
	if err != nil {
		return
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		if line := scanner.Text(); line != "" && !strings.HasPrefix(line, "#") {
			fn(strings.Split(line, ":"))
		}
	}
}
