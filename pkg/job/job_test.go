// pkg/job/job_test.go
package job

import (
	"os"
	"path/filepath"
	"testing"
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
		{"disabled", "# * * * * * root /usr/bin/true", true, false, "* * * * *", "root", "/usr/bin/true"},
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
		{"disabled", "# * * * * * backup.sh", true, false, "* * * * *", "backup.sh"},
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
		"# 0 0 * * * root /usr/bin/false\n" +
		"\n" +
		"# just a comment\n"
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	jobs, warnings := parseFile(path, true, "")
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
	jobs, warnings := parseFile(filepath.Join(t.TempDir(), "missing"), true, "")
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
