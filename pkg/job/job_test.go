// pkg/job/job_test.go
package job

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestTakeFields(t *testing.T) {
	for _, test := range []struct {
		raw        string
		n          int
		wantFields []string
		wantRest   string
		wantOK     bool
	}{
		{"a b c d", 3, []string{"a", "b", "c"}, "d", true},
		{"a b c", 3, nil, "", false},
		{"a b c d e", 2, []string{"a", "b"}, "c d e", true},
		{"", 1, nil, "", false},
	} {
		fields, rest, ok := takeFields(test.raw, test.n)
		if ok != test.wantOK {
			t.Fatalf("takeFields(%q, %d) ok = %t, want %t", test.raw, test.n, ok, test.wantOK)
		}
		if !ok {
			continue
		}
		if rest != test.wantRest || len(fields) != len(test.wantFields) {
			t.Fatalf("takeFields(%q, %d) = %q, %q, want %q, %q", test.raw, test.n, fields, rest, test.wantFields, test.wantRest)
		}
		for i := range fields {
			if fields[i] != test.wantFields[i] {
				t.Fatalf("takeFields(%q, %d) fields = %q, want %q", test.raw, test.n, fields, test.wantFields)
			}
		}
	}
}

func TestValidSchedule(t *testing.T) {
	for _, test := range []struct {
		fields []string
		want   bool
	}{
		{[]string{"*", "*", "*", "*", "*"}, true},
		{[]string{"*/15", "0-5", "1,15,30", "*", "*"}, true},
		{[]string{"this", "is", "a", "comment", "line"}, false},
		{[]string{"@daily"}, true},
		{[]string{"@bogus"}, false},
	} {
		if got := validSchedule(test.fields); got != test.want {
			t.Errorf("validSchedule(%q) = %t, want %t", test.fields, got, test.want)
		}
	}
}

func TestParseCrontabLine(t *testing.T) {
	for _, test := range []struct {
		name         string
		raw          string
		wantOK       bool
		wantEnabled  bool
		wantSchedule string
		wantUser     string
		wantCommand  string
	}{
		{"valid", "* * * * * root /usr/bin/true", true, true, "* * * * *", "root", "/usr/bin/true"},
		{"macro", "@daily root backup.sh", true, true, "@daily", "root", "backup.sh"},
		{"disabled", "#[seer-disabled] * * * * * root /usr/bin/true", true, false, "* * * * *", "root", "/usr/bin/true"},
		{"cron-looking comment", "# 0 0 * * * root /usr/bin/true", false, false, "", "", ""},
		{"prose comment", "# this is a much longer comment line", false, false, "", "", ""},
		{"too few fields", "* * * * root", false, false, "", "", ""},
		{"bad macro", "@bogus root command", false, false, "", "", ""},
		{"blank", "", false, false, "", "", ""},
	} {
		t.Run(test.name, func(t *testing.T) {
			schedule, user, command, enabled, ok := parseCrontabLine(test.raw)
			if ok != test.wantOK {
				t.Fatalf("ok = %t, want %t", ok, test.wantOK)
			}
			if !ok {
				return
			}
			if schedule != test.wantSchedule || user != test.wantUser || command != test.wantCommand || enabled != test.wantEnabled {
				t.Fatalf("got (%q, %q, %q, %t), want (%q, %q, %q, %t)",
					schedule, user, command, enabled,
					test.wantSchedule, test.wantUser, test.wantCommand, test.wantEnabled)
			}
		})
	}
}

func TestParseSpoolLine(t *testing.T) {
	for _, test := range []struct {
		name         string
		raw          string
		wantOK       bool
		wantEnabled  bool
		wantSchedule string
		wantCommand  string
	}{
		{"valid", "*/15 * * * * backup.sh", true, true, "*/15 * * * *", "backup.sh"},
		{"disabled", "#[seer-disabled] * * * * * backup.sh", true, false, "* * * * *", "backup.sh"},
		{"prose comment", "# this is a much longer comment line", false, false, "", ""},
		{"too few fields", "* * * *", false, false, "", ""},
	} {
		t.Run(test.name, func(t *testing.T) {
			schedule, command, enabled, ok := parseSpoolLine(test.raw)
			if ok != test.wantOK {
				t.Fatalf("ok = %t, want %t", ok, test.wantOK)
			}
			if !ok {
				return
			}
			if schedule != test.wantSchedule || command != test.wantCommand || enabled != test.wantEnabled {
				t.Fatalf("got (%q, %q, %t), want (%q, %q, %t)",
					schedule, command, enabled, test.wantSchedule, test.wantCommand, test.wantEnabled)
			}
		})
	}
}

func TestJobIDStableAndDistinct(t *testing.T) {
	a := jobID("/etc/crontab", "* * * * * root /usr/bin/true")
	b := jobID("/etc/crontab", "* * * * * root /usr/bin/true")
	if a != b {
		t.Fatalf("jobID not stable: %q != %q", a, b)
	}
	c := jobID("/etc/crontab", "* * * * * root /usr/bin/false")
	if a == c {
		t.Fatalf("jobID collided for different raw lines")
	}
}

func TestParseFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "crontab")
	content := "MAILTO=root\n" +
		"* * * * * root /usr/bin/true\n" +
		"#[seer-disabled] 0 0 * * * root /usr/bin/false\n" +
		"# 5 5 * * * root /usr/bin/documented-but-not-a-job\n" +
		"\n" +
		"# just a comment\n"
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	jobs, warnings := parseFile(path, true, "", KindCronD)
	if len(warnings) != 0 {
		t.Fatalf("unexpected warnings: %v", warnings)
	}
	if len(jobs) != 2 {
		t.Fatalf("got %d jobs, want 2: %+v", len(jobs), jobs)
	}
	if jobs[0].LineNumber != 2 || !jobs[0].Enabled {
		t.Fatalf("unexpected first job: %+v", jobs[0])
	}
	if jobs[1].LineNumber != 3 || jobs[1].Enabled {
		t.Fatalf("unexpected second job: %+v", jobs[1])
	}
}

func TestParseFileMissingReportsWarning(t *testing.T) {
	jobs, warnings := parseFile(filepath.Join(t.TempDir(), "missing"), true, "", KindCronD)
	if jobs != nil {
		t.Fatalf("expected no jobs, got %+v", jobs)
	}
	if len(warnings) != 1 {
		t.Fatalf("expected one warning, got %v", warnings)
	}
}

func TestRegularFilesRecursesAndSorts(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "nested"), 0755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"b", "nested/a", "a"} {
		if err := os.WriteFile(filepath.Join(dir, name), nil, 0644); err != nil {
			t.Fatal(err)
		}
	}

	files, warnings := regularFiles(dir)
	if len(warnings) != 0 {
		t.Fatalf("unexpected warnings: %v", warnings)
	}
	want := []string{
		filepath.Join(dir, "a"),
		filepath.Join(dir, "b"),
		filepath.Join(dir, "nested/a"),
	}
	if len(files) != len(want) {
		t.Fatalf("got %q, want %q", files, want)
	}
	for i := range want {
		if files[i] != want[i] {
			t.Fatalf("got %q, want %q", files, want)
		}
	}
}

func TestJobStringAndDescribe(t *testing.T) {
	j := Job{
		ID: "abc123", Source: "/etc/crontab", User: "root",
		Schedule: "* * * * *", Command: "/usr/bin/true",
		LineNumber: 2, Raw: "* * * * * root /usr/bin/true", Enabled: true,
	}
	want := `[abc123] * * * * * root "/usr/bin/true"` + "\n"
	if got := j.String(); got != want {
		t.Fatalf("String() = %q, want %q", got, want)
	}

	j.Enabled = false
	if got := j.String(); got != want[:len(want)-1]+" [DISABLED]\n" {
		t.Fatalf("disabled String() = %q", got)
	}
}

func TestNamedFieldsAndDuplicateIDs(t *testing.T) {
	if !validSchedule([]string{"0", "9", "*", "JAN", "mon-fri"}) {
		t.Fatal("named month/weekday schedule should be valid")
	}
	path := filepath.Join(t.TempDir(), "crontab")
	line := "0 9 * * * echo hi\n"
	if err := os.WriteFile(path, []byte(line+line), 0o600); err != nil {
		t.Fatal(err)
	}
	jobs, _ := parseFile(path, false, "u", KindUser)
	if len(jobs) != 2 || jobs[0].ID == jobs[1].ID {
		t.Fatalf("duplicate lines must get distinct IDs, got %+v", jobs)
	}
	if jobs[0].ID != jobID(path, "0 9 * * * echo hi") {
		t.Fatal("first occurrence must keep its original ID")
	}
}

func TestScheduleRole(t *testing.T) {
	for schedule, want := range map[string]Role{
		"@reboot": RoleReboot, "@daily": RoleMacro,
		"* * * * *": RoleFrequent, "*/5 * * * *": RoleFrequent, "0 3 * * *": RoleStandard,
	} {
		if got := (Job{Schedule: schedule}).ScheduleRole(); got != want {
			t.Errorf("ScheduleRole(%q) = %q, want %q", schedule, got, want)
		}
	}
	if (Job{User: "root"}).UserRole() != RoleUserRoot {
		t.Error("root should be RoleUserRoot")
	}
}

func TestNextRun(t *testing.T) {
	from := time.Date(2026, 10, 3, 10, 7, 30, 0, time.UTC) // a Saturday
	for schedule, want := range map[string]string{
		"30 9 * * *":      "2026-10-04 09:30",
		"*/15 * * * *":    "2026-10-03 10:15",
		"0 0 1 * *":       "2026-11-01 00:00",
		"0 9 * * mon-fri": "2026-10-05 09:00",
		"0 0 29 2 *":      "2028-02-29 00:00",
		"@hourly":         "2026-10-03 11:00",
		"@weekly":         "2026-10-04 00:00",
		"0 0 1 1 *":       "2027-01-01 00:00",
		"0 0 */2 * 1":     "2026-10-05 00:00", // star-prefixed field: AND, not OR
		"0 0 1 * 1":       "2026-10-05 00:00", // restricted both: OR
		"0 0 * * 7":       "2026-10-04 00:00",
	} {
		got, ok := NextRun(schedule, from)
		if !ok || got.Format("2006-01-02 15:04") != want {
			t.Errorf("NextRun(%q) = %v, %t, want %s", schedule, got, ok, want)
		}
	}
	for _, schedule := range []string{"@reboot", "@every-3d", "bogus", "0 0 31 2 *", "61 * * * *"} {
		if _, ok := NextRun(schedule, from); ok {
			t.Errorf("NextRun(%q) should not resolve", schedule)
		}
	}
}

func TestAnacronAndPeriodic(t *testing.T) {
	path := filepath.Join(t.TempDir(), "anacrontab")
	content := "SHELL=/bin/sh\n1 5 cron.daily run-parts /etc/cron.daily\n3 10 odd /opt/odd.sh\n@monthly 45 m /opt/m.sh\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	jobs, warnings := anacronJobs(path)
	if len(warnings) != 0 || len(jobs) != 3 {
		t.Fatalf("got %+v, %v", jobs, warnings)
	}
	if jobs[0].Schedule != "@daily" || jobs[1].Schedule != "@every-3d" || jobs[2].Schedule != "@monthly" {
		t.Fatalf("unexpected schedules: %+v", jobs)
	}
	for _, j := range jobs {
		if !j.ReadOnly || j.User != "root" || j.Kind != KindAnacron {
			t.Fatalf("anacron job not read-only root: %+v", j)
		}
	}
	if got := DescribeSchedule("@every-3d"); got != "Every 3 days (anacron)" {
		t.Fatalf("got %q", got)
	}
}

func TestJobRisks(t *testing.T) {
	dir := t.TempDir()
	script := filepath.Join(dir, "run.sh")
	if err := os.WriteFile(script, []byte("#!/bin/sh\n"), 0o666); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(script, 0o777); err != nil {
		t.Fatal(err)
	}

	risky := Job{User: "root", Enabled: true, Command: "FOO=1 " + script + " --arg"}
	if len(jobRisks(risky)) == 0 {
		t.Fatal("writable script run by root should be flagged")
	}
	if got := jobRisks(Job{User: "root", Enabled: true, Command: "/tmp/x.sh"}); len(got) == 0 {
		t.Fatal("temp directory command should be flagged")
	}
	if jobRisks(Job{User: "root", Enabled: false, Command: script}) != nil {
		t.Fatal("disabled jobs are not flagged")
	}
	if jobRisks(Job{User: "alice", Enabled: true, Command: script}) != nil {
		t.Fatal("non-root jobs are not flagged")
	}
	if jobRisks(Job{User: "root", Enabled: true, Command: "backup --now"}) != nil {
		t.Fatal("PATH-resolved commands are not checked")
	}
}

func TestDescribeShowsNextRunAndRisk(t *testing.T) {
	now = func() time.Time { return time.Date(2026, 10, 3, 10, 0, 0, 0, time.UTC) }
	defer func() { now = time.Now }()

	j := Job{ID: "x", Source: "/etc/crontab", User: "root", Schedule: "30 11 * * *",
		Command: "/tmp/a.sh", LineNumber: 1, Raw: "r", Enabled: true, Kind: KindCrontab,
		Risks: []string{"runs from temporary directory /tmp"}}
	got := j.Describe()
	for _, want := range []string{"├ Type: crontab", "├ Next run: 2026-10-03 11:30 (in 1h 30m)", "├ Risk: runs from temporary directory /tmp"} {
		if !strings.Contains(got, want) {
			t.Errorf("Describe() missing %q in:\n%s", want, got)
		}
	}
	if !strings.Contains(j.String(), "[RISK]") {
		t.Error("String() should show [RISK]")
	}
}
