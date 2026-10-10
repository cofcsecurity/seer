package job

import (
	"os"
	"os/user"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func write(t *testing.T, path, content string, mode os.FileMode) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), mode); err != nil {
		t.Fatal(err)
	}
}

func currentUser(t *testing.T) string {
	t.Helper()
	u, err := user.Current()
	if err != nil {
		t.Skip("no current user")
	}
	return u.Username
}

func TestSystemdTimers(t *testing.T) {
	useTempCron(t)
	dir := systemdDirs[0]

	write(t, filepath.Join(dir, "backup.timer"), "[Timer]\nOnCalendar=daily\nOnCalendar=*-*-* 12:00:00\n[Install]\nWantedBy=timers.target\n", 0o644)
	write(t, filepath.Join(dir, "backup.service"), "[Service]\nUser=svc\nExecStart=-/usr/local/bin/backup.sh --full\n", 0o644)
	write(t, filepath.Join(dir, "poll.timer"), "[Timer]\nOnUnitActiveSec=2min\n[Install]\nWantedBy=timers.target\n", 0o644)
	write(t, filepath.Join(dir, "static.timer"), "[Timer]\nOnBootSec=5min\nUnit=other.service\n", 0o644)
	if err := os.Symlink("/dev/null", filepath.Join(dir, "off.timer")); err != nil {
		t.Fatal(err)
	}
	// Only backup.timer is linked into a target.
	if err := os.MkdirAll(filepath.Join(dir, "timers.target.wants"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(dir, "backup.timer"), filepath.Join(dir, "timers.target.wants", "backup.timer")); err != nil {
		t.Fatal(err)
	}

	byUnit := map[string]Job{}
	jobs, _ := ListJobs()
	for _, j := range jobs {
		if j.Kind == KindTimer {
			byUnit[j.Unit] = j
		}
	}
	if len(byUnit) != 4 {
		t.Fatalf("want 4 timers, got %+v", byUnit)
	}

	b := byUnit["backup.timer"]
	if !b.Enabled || b.ReadOnly || b.User != "svc" || b.Command != "/usr/local/bin/backup.sh --full" ||
		b.Schedule != "OnCalendar=daily; OnCalendar=*-*-* 12:00:00" {
		t.Fatalf("backup timer wrong: %+v", b)
	}
	if p := byUnit["poll.timer"]; p.Enabled || p.ReadOnly || p.ScheduleRole() != RoleFrequent {
		t.Fatalf("poll timer should be disabled, editable and frequent: %+v", p)
	}
	if s := byUnit["static.timer"]; !s.ReadOnly || !s.Enabled || !strings.Contains(s.Note, "static") {
		t.Fatalf("static timer wrong: %+v", s)
	}
	if m := byUnit["off.timer"]; !m.ReadOnly || !strings.Contains(m.Note, "masked") {
		t.Fatalf("masked timer wrong: %+v", m)
	}

	// Disable and enable go through systemctl; nothing is backed up.
	var ran []string
	runCommand = func(name string, args ...string) ([]byte, error) {
		ran = append(ran, name+" "+strings.Join(args, " "))
		return nil, nil
	}
	if backup, err := Disable(b.ID); err != nil || backup != "" {
		t.Fatalf("disable timer = %q, %v", backup, err)
	}
	if _, err := Enable(byUnit["poll.timer"].ID); err != nil {
		t.Fatal(err)
	}
	if want := []string{"systemctl disable --now backup.timer", "systemctl enable --now poll.timer"}; strings.Join(ran, "|") != strings.Join(want, "|") {
		t.Fatalf("ran %v, want %v", ran, want)
	}
	if _, err := Enable(b.ID); err == nil {
		t.Fatal("enabling an enabled timer should fail")
	}
	if _, err := Remove(b.ID); err == nil {
		t.Fatal("removing a timer should be refused")
	}
	if _, err := Disable(byUnit["static.timer"].ID); err == nil {
		t.Fatal("static timers cannot be disabled")
	}
	if diff, err := Diff(OpDisable, b.ID); err != nil || diff != "Would run: systemctl disable --now backup.timer\n" {
		t.Fatalf("diff = %q, %v", diff, err)
	}
}

func TestAtJobsAreListedAndNotMistakenForCrontabs(t *testing.T) {
	_, _, spool := useTempCron(t)

	when := time.Date(2026, 10, 5, 14, 30, 0, 0, time.Local)
	name := "a00007" + strings.ToLower(strings.Repeat("0", 8-len(hex(when.Unix()/60)))) + hex(when.Unix()/60)
	script := "#!/bin/sh\n# atrun uid=0 gid=0\n# mail root 0\numask 22\nPATH=/usr/bin; export PATH\ncd /root || {\n\t echo 'Execution directory inaccessible' >&2\n\t exit 1\n}\n/opt/cleanup.sh --all\n"
	write(t, filepath.Join(spool, "atjobs", name), script, 0o700)
	write(t, filepath.Join(spool, "atjobs", ".SEQ"), "00007\n", 0o600)
	write(t, filepath.Join(spool, "alice"), "0 6 * * * ~/x.sh\n", 0o600)

	jobs, _ := ListJobs()
	var at []Job
	var cronUsers []string
	for _, j := range jobs {
		switch j.Kind {
		case KindAt:
			at = append(at, j)
		case KindUser:
			cronUsers = append(cronUsers, j.User)
		}
	}
	if len(cronUsers) != 1 || cronUsers[0] != "alice" {
		t.Fatalf("at spool files must not be read as user crontabs, got %v", cronUsers)
	}
	if len(at) != 1 {
		t.Fatalf("want one at job, got %+v", at)
	}
	j := at[0]
	if j.User != "root" || j.Command != "/opt/cleanup.sh --all" || j.Schedule != "at 2026-10-05 14:30" || !j.ReadOnly || j.Unit != "7" {
		t.Fatalf("at job wrong: %+v", j)
	}
	if _, err := Remove(j.ID); err == nil {
		t.Fatal("at jobs are read-only")
	}
}

func hex(n int64) string {
	const digits = "0123456789abcdef"
	if n == 0 {
		return "0"
	}
	var b []byte
	for ; n > 0; n /= 16 {
		b = append([]byte{digits[n%16]}, b...)
	}
	return string(b)
}

func TestEnvAccessAndRecent(t *testing.T) {
	crontab, _, spool := useTempCron(t)

	write(t, crontab, "SHELL=/bin/bash\nMAILTO=root\n0 1 * * * root /bin/a\nMAILTO=ops\nPATH=/usr/bin\n0 2 * * * root /bin/b\n", 0o644)
	write(t, filepath.Join(spool, "mallory"), "0 3 * * * /bin/c\n", 0o600)
	write(t, filepath.Join(spool, "alice"), "0 4 * * * /bin/d\n", 0o600)
	write(t, cronDenyPath, "# denied\nmallory\n", 0o644)

	a := listByCommand(t, "/bin/a")[0]
	b := listByCommand(t, "/bin/b")[0]
	if strings.Join(a.Env, ",") != "SHELL=/bin/bash,MAILTO=root" {
		t.Fatalf("a env = %v", a.Env)
	}
	if strings.Join(b.Env, ",") != "SHELL=/bin/bash,MAILTO=ops,PATH=/usr/bin" {
		t.Fatalf("b env = %v (later assignments replace earlier ones and apply only below them)", b.Env)
	}
	if !listByCommand(t, "/bin/c")[0].Denied || listByCommand(t, "/bin/d")[0].Denied {
		t.Fatal("only mallory is denied")
	}

	// With cron.allow present, only listed users may have crontabs.
	write(t, cronAllowPath, "alice\n", 0o644)
	if listByCommand(t, "/bin/d")[0].Denied || !listByCommand(t, "/bin/c")[0].Denied {
		t.Fatal("cron.allow should override cron.deny")
	}

	// Recent: file modification times against a fixed clock.
	old := time.Now().Add(-72 * time.Hour)
	if err := os.Chtimes(crontab, old, old); err != nil {
		t.Fatal(err)
	}
	if listByCommand(t, "/bin/a")[0].IsRecent() {
		t.Fatal("a 3-day-old file is not recent")
	}
	if !listByCommand(t, "/bin/d")[0].IsRecent() {
		t.Fatal("a just-written file is recent")
	}
	RecentWindow = 96 * time.Hour
	defer func() { RecentWindow = 24 * time.Hour }()
	if !listByCommand(t, "/bin/a")[0].IsRecent() {
		t.Fatal("a wider window should include it")
	}
}

func TestHistory(t *testing.T) {
	crontab, _, _ := useTempCron(t)
	write(t, crontab, "0 1 * * * root /usr/local/bin/backup.sh\n", 0o644)
	j := listByCommand(t, "/usr/local/bin/backup.sh")[0]

	log := filepath.Join(t.TempDir(), "syslog")
	write(t, log, strings.Join([]string{
		"Oct  3 01:00:01 host CRON[1]: (root) CMD (/usr/local/bin/backup.sh)",
		"Oct  3 01:05:01 host CRON[2]: (root) CMD (/other)",
		"Oct  4 01:00:01 host CRON[3]: (root) CMD (/usr/local/bin/backup.sh)",
		"Oct  4 01:00:01 host CRON[4]: (alice) CMD (/usr/local/bin/backup.sh)",
	}, "\n"), 0o644)
	logFiles = []string{log}

	h, err := JobHistory(j, 5)
	if err != nil || h.Source != log || len(h.Entries) != 2 || !strings.Contains(h.Entries[1], "Oct  4") {
		t.Fatalf("history = %+v, %v", h, err)
	}
	if h, _ := JobHistory(j, 1); len(h.Entries) != 1 || !strings.Contains(h.Entries[0], "Oct  4") {
		t.Fatalf("limit should keep the newest run: %+v", h)
	}

	// Timers read the journal for their service.
	var args string
	runCommand = func(name string, a ...string) ([]byte, error) {
		args = name + " " + strings.Join(a, " ")
		return []byte("-- Journal begins --\n2026-10-04T01:00:00 host systemd[1]: Started Backup.\n2026-10-04T01:00:02 host backup[9]: done\n"), nil
	}
	timer := Job{Kind: KindTimer, Unit: "backup.timer", Service: "backup.service", Command: "/usr/local/bin/backup.sh"}
	h, err = JobHistory(timer, 5)
	if err != nil || len(h.Entries) != 2 || !strings.Contains(args, "-u backup.service") {
		t.Fatalf("timer history = %+v (%q), %v", h, args, err)
	}
	if _, err := JobHistory(Job{Kind: KindAt}, 5); err == nil {
		t.Fatal("at jobs have no history")
	}
}

func TestCheckFindsDuplicatesAndSimilarJobs(t *testing.T) {
	jobs := []Job{
		{ID: "1", User: "root", Schedule: "0 1 * * *", Command: "/bin/x  --go", Enabled: true},
		{ID: "2", User: "root", Schedule: "0 1 * * *", Command: "/bin/x --go", Enabled: true},
		{ID: "3", User: "root", Schedule: "0 5 * * *", Command: "/bin/x --go", Enabled: true},
		{ID: "4", User: "root", Schedule: "0 1 * * *", Command: "/bin/off", Enabled: false},
		{ID: "5", User: "root", Schedule: "0 1 * * *", Command: "/bin/off", Enabled: true},
	}
	var dup, similar []Finding
	for _, f := range Check(jobs) {
		switch f.Kind {
		case "duplicate":
			dup = append(dup, f)
		case "similar":
			similar = append(similar, f)
		}
	}
	if len(dup) != 1 || strings.Join(dup[0].Jobs, ",") != "1,2" {
		t.Fatalf("duplicates = %+v", dup)
	}
	if len(similar) != 1 || strings.Join(similar[0].Jobs, ",") != "1,2,3" {
		t.Fatalf("similar = %+v (disabled jobs must be ignored)", similar)
	}
}

func TestCheckFindsCommandsAnotherUserCanChange(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("needs a non-root file owner")
	}
	me := currentUser(t)

	script := filepath.Join(t.TempDir(), "run.sh")
	write(t, script, "#!/bin/sh\n", 0o755)

	jobs := []Job{
		{ID: "r", User: "root", Schedule: "0 1 * * *", Command: script, Enabled: true},
		{ID: "m", User: me, Schedule: "0 2 * * *", Command: "/bin/true", Enabled: true},
	}
	var found []Finding
	for _, f := range Check(jobs) {
		if f.Kind == "shared-write" {
			found = append(found, f)
		}
	}
	if len(found) == 0 || found[0].Jobs[0] != "r" || found[0].Jobs[1] != "m" || !strings.Contains(found[0].Message, me+" can change") {
		t.Fatalf("findings = %+v", found)
	}

	// Without a job of their own, the file owner is not reported.
	if got := Check(jobs[:1]); len(got) != 0 {
		t.Fatalf("no second account with jobs, want no findings: %+v", got)
	}
}

func TestAddJob(t *testing.T) {
	crontab, cronD, spool := useTempCron(t)
	me := currentUser(t)

	// Validation.
	for name, spec := range map[string]AddSpec{
		"bad schedule":     {User: me, Schedule: "61 * * * *", Command: "/bin/x", File: crontab},
		"short schedule":   {User: me, Schedule: "* * *", Command: "/bin/x", File: crontab},
		"empty command":    {User: me, Schedule: "@daily", Command: " ", File: crontab},
		"percent":          {User: me, Schedule: "@daily", Command: "date +%F", File: crontab},
		"newline":          {User: me, Schedule: "@daily", Command: "a\nb", File: crontab},
		"unknown user":     {User: "no-such-user-xyz", Schedule: "@daily", Command: "/bin/x", File: crontab},
		"not a cron file":  {User: me, Schedule: "@daily", Command: "/bin/x", File: "/etc/passwd"},
		"ignored name":     {User: me, Schedule: "@daily", Command: "/bin/x", File: filepath.Join(cronD, ".hidden")},
		"no user crontab":  {User: me, Schedule: "@daily", Command: "/bin/x"},
		"wrong spool user": {User: me, Schedule: "@daily", Command: "/bin/x", File: filepath.Join(spool, "someone")},
	} {
		if _, err := PlanAdd(spec); err == nil {
			t.Errorf("%s: want an error", name)
		}
	}
	if _, err := PlanAdd(AddSpec{User: me, Schedule: "@daily", Command: `date +\%F`, File: crontab}); err != nil {
		t.Errorf("escaped percent should be accepted: %v", err)
	}

	// Append to a user's existing crontab (no trailing newline in the file).
	write(t, filepath.Join(spool, me), "0 6 * * * /bin/old", 0o600)
	plan, err := PlanAdd(AddSpec{User: me, Schedule: "30  2 * * 1-5", Command: "/bin/new --go"})
	if err != nil || plan.Line != "30 2 * * 1-5 /bin/new --go" || !plan.Exists {
		t.Fatalf("plan = %+v, %v", plan, err)
	}
	if !strings.Contains(AddDiff(plan), "+30 2 * * 1-5 /bin/new --go") {
		t.Fatalf("diff = %q", AddDiff(plan))
	}
	id, backup, err := Add(plan)
	if err != nil || id == "" {
		t.Fatalf("add = %q, %v", id, err)
	}
	data, _ := os.ReadFile(filepath.Join(spool, me))
	if string(data) != "0 6 * * * /bin/old\n30 2 * * 1-5 /bin/new --go\n" {
		t.Fatalf("file = %q", data)
	}
	if _, err := PlanAdd(AddSpec{User: me, Schedule: "30 2 * * 1-5", Command: "/bin/new --go"}); err == nil {
		t.Fatal("adding the same job twice should fail")
	}

	// Undo removes just that line.
	if _, err := Restore(BackupID(backup), false); err != nil {
		t.Fatal(err)
	}
	if data, _ := os.ReadFile(filepath.Join(spool, me)); string(data) != "0 6 * * * /bin/old\n" {
		t.Fatalf("after undo: %q", data)
	}

	// A new /etc/cron.d file is created, and undoing the add removes it.
	file := filepath.Join(cronD, "app")
	plan, err = PlanAdd(AddSpec{User: me, Schedule: "@daily", Command: "/tmp/x.sh", File: file})
	if err != nil || plan.Exists || plan.Line != "@daily "+me+" /tmp/x.sh" {
		t.Fatalf("plan = %+v, %v", plan, err)
	}
	id, backup, err = Add(plan)
	if err != nil || id == "" {
		t.Fatalf("add = %q, %v", id, err)
	}
	if info, err := os.Stat(file); err != nil || info.Mode().Perm() != 0o644 {
		t.Fatalf("created file: %v, %v", info, err)
	}
	if _, err := Restore(BackupID(backup), false); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(file); !os.IsNotExist(err) {
		t.Fatal("undoing an add that created the file should delete it")
	}
}
