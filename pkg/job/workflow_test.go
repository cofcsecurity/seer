package job

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// useTempCron points the scanner at an isolated tree and returns its paths.
func useTempCron(t *testing.T) (crontab, cronD, spool string) {
	t.Helper()

	root := t.TempDir()
	crontab = filepath.Join(root, "crontab")
	cronD = filepath.Join(root, "cron.d")
	spool = filepath.Join(root, "spool")

	for _, dir := range []string{cronD, spool} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}

	oldCrontab, oldCronD, oldAnacron, oldSpool, oldPeriodic := crontabPath, cronDDir, anacrontabPath, spoolRoots, periodicDirs
	oldSystemd, oldAt, oldAllow, oldDeny, oldLogs, oldRun := systemdDirs, atSpoolDirs, cronAllowPath, cronDenyPath, logFiles, runCommand
	crontabPath, cronDDir, anacrontabPath, spoolRoots = crontab, cronD, filepath.Join(root, "anacrontab"), []string{spool}
	periodicDirs = nil
	systemdDirs = []string{filepath.Join(root, "systemd")}
	atSpoolDirs = []string{filepath.Join(spool, "atjobs")}
	cronAllowPath, cronDenyPath = filepath.Join(root, "cron.allow"), filepath.Join(root, "cron.deny")
	logFiles = nil
	runCommand = func(string, ...string) ([]byte, error) { return nil, fmt.Errorf("no commands in tests") }
	t.Cleanup(func() {
		crontabPath, cronDDir, anacrontabPath, spoolRoots, periodicDirs = oldCrontab, oldCronD, oldAnacron, oldSpool, oldPeriodic
		systemdDirs, atSpoolDirs, cronAllowPath, cronDenyPath, logFiles, runCommand = oldSystemd, oldAt, oldAllow, oldDeny, oldLogs, oldRun
	})

	return crontab, cronD, spool
}

func listByCommand(t *testing.T, command string) []Job {
	t.Helper()

	jobs, _ := ListJobs()
	var out []Job
	for _, j := range jobs {
		if j.Command == command {
			out = append(out, j)
		}
	}

	return out
}

func TestDisableEnableRoundTrip(t *testing.T) {
	crontab, _, spool := useTempCron(t)

	original := "# system crontab\nMAILTO=root\n30 2 * * 1-5 root /usr/local/bin/backup.sh\n0 4 * * * root /usr/local/bin/other.sh\n"
	if err := os.WriteFile(crontab, []byte(original), 0o644); err != nil {
		t.Fatal(err)
	}
	userLine := "*/10 * * * * ~/sync.sh\r\n"
	if err := os.WriteFile(filepath.Join(spool, "alice"), []byte(userLine), 0o600); err != nil {
		t.Fatal(err)
	}

	for _, command := range []string{"/usr/local/bin/backup.sh", "~/sync.sh"} {
		jobs := listByCommand(t, command)
		if len(jobs) != 1 || !jobs[0].Enabled {
			t.Fatalf("%s: want one enabled job, got %+v", command, jobs)
		}
		id, source := jobs[0].ID, jobs[0].Source
		before, _ := os.ReadFile(source)

		// Disable: the job must still be listed, as disabled, with the same ID.
		backup, err := Disable(id)
		if err != nil {
			t.Fatalf("%s: disable: %v", command, err)
		}
		jobs = listByCommand(t, command)
		if len(jobs) != 1 || jobs[0].Enabled || jobs[0].ID != id {
			t.Fatalf("%s: after disable want one disabled job with ID %s, got %+v", command, id, jobs)
		}
		if !strings.Contains(jobs[0].Raw, DisabledMarker) {
			t.Fatalf("%s: disabled line lacks marker: %q", command, jobs[0].Raw)
		}

		// The backup must hold the pre-change file and must not be listed as a job.
		if data, _ := os.ReadFile(backup); string(data) != string(before) {
			t.Fatalf("%s: backup differs from original", command)
		}
		all, _ := ListJobs()
		for _, j := range all {
			if strings.Contains(j.Source, ".seer-job-") || strings.Contains(j.User, ".seer-job-") {
				t.Fatalf("%s: backup file listed as a job: %+v", command, j)
			}
		}
		if _, err := Disable(id); err == nil {
			t.Fatalf("%s: disabling twice should fail", command)
		}

		// Enable: back to the exact original bytes, same ID.
		if _, err := Enable(id); err != nil {
			t.Fatalf("%s: enable: %v", command, err)
		}
		if after, _ := os.ReadFile(source); string(after) != string(before) {
			t.Fatalf("%s: enable did not restore the file:\n%q\nwant\n%q", command, after, before)
		}
		jobs = listByCommand(t, command)
		if len(jobs) != 1 || !jobs[0].Enabled || jobs[0].ID != id {
			t.Fatalf("%s: after enable want enabled job with ID %s, got %+v", command, id, jobs)
		}
		if _, err := Enable(id); err == nil {
			t.Fatalf("%s: enabling twice should fail", command)
		}
	}

	// Remove drops only the chosen line.
	jobs := listByCommand(t, "/usr/local/bin/other.sh")
	if _, err := Remove(jobs[0].ID); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(crontab)
	if strings.Contains(string(data), "other.sh") || !strings.Contains(string(data), "backup.sh") {
		t.Fatalf("remove touched the wrong line:\n%s", data)
	}
}

func TestOrdinaryCommentsAreNotJobs(t *testing.T) {
	crontab, _, _ := useTempCron(t)

	content := "# 0 5 * * * root /usr/bin/documented-example\n0 6 * * * root /usr/bin/real\n"
	if err := os.WriteFile(crontab, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	jobs, _ := ListJobs()
	if len(jobs) != 1 || jobs[0].Command != "/usr/bin/real" {
		t.Fatalf("only the real job should be listed, got %+v", jobs)
	}
}

func TestDuplicateLinesStayIndependent(t *testing.T) {
	crontab, _, _ := useTempCron(t)

	line := "0 9 * * * root /usr/bin/twice\n"
	if err := os.WriteFile(crontab, []byte(line+line), 0o644); err != nil {
		t.Fatal(err)
	}
	jobs := listByCommand(t, "/usr/bin/twice")
	if len(jobs) != 2 {
		t.Fatalf("want 2 jobs, got %+v", jobs)
	}
	if _, err := Disable(jobs[1].ID); err != nil {
		t.Fatal(err)
	}

	data, _ := os.ReadFile(crontab)
	if string(data) != line+DisabledMarker+" "+line {
		t.Fatalf("wrong line disabled:\n%s", data)
	}
	after := listByCommand(t, "/usr/bin/twice")
	if len(after) != 2 || !after[0].Enabled || after[1].Enabled {
		t.Fatalf("want one enabled then one disabled, got %+v", after)
	}
	if after[0].ID == after[1].ID {
		t.Fatal("IDs must stay distinct")
	}
}

func TestRestoreUndoesRemoveAndIsItselfUndoable(t *testing.T) {
	crontab, _, _ := useTempCron(t)

	original := "0 6 * * * root /usr/bin/keep\n0 7 * * * root /usr/bin/doomed\n"
	if err := os.WriteFile(crontab, []byte(original), 0o640); err != nil {
		t.Fatal(err)
	}

	doomed := listByCommand(t, "/usr/bin/doomed")[0]
	backup, err := Remove(doomed.ID)
	if err != nil {
		t.Fatal(err)
	}

	backups, _ := Backups()
	if len(backups) != 1 || backups[0].Path != backup || backups[0].Op != "remove" || backups[0].Source != crontab {
		t.Fatalf("unexpected backups: %+v", backups)
	}
	if backups[0].ID != BackupID(backup) {
		t.Fatal("BackupID must match the listed ID")
	}

	diff, err := RestoreDiff(backups[0].ID, false)
	if err != nil || !strings.Contains(diff, "+0 7 * * * root /usr/bin/doomed") || strings.Contains(diff, "-0 6") {
		t.Fatalf("diff should only add the removed line: %q, %v", diff, err)
	}

	undo, err := Restore(backups[0].ID, false)
	if err != nil {
		t.Fatal(err)
	}
	if data, _ := os.ReadFile(crontab); string(data) != original {
		t.Fatalf("restore did not bring the file back:\n%s", data)
	}
	if info, _ := os.Stat(crontab); info.Mode().Perm() != 0o640 {
		t.Fatalf("restore changed the mode to %v", info.Mode().Perm())
	}
	if _, err := Restore(backups[0].ID, false); err == nil {
		t.Fatal("restoring an identical file should be rejected")
	}

	// Undo the restore: the removed state comes back.
	if _, err := Restore(BackupID(undo), false); err != nil {
		t.Fatal(err)
	}
	if data, _ := os.ReadFile(crontab); strings.Contains(string(data), "doomed") {
		t.Fatalf("undoing the restore should re-remove the job:\n%s", data)
	}
}

func TestCleanupSelectsByAge(t *testing.T) {
	crontab, _, _ := useTempCron(t)
	if err := os.WriteFile(crontab, []byte("0 6 * * * root /usr/bin/a\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	id := listByCommand(t, "/usr/bin/a")[0].ID
	oldBackup, _ := Disable(id)
	newBackup, _ := Enable(id)

	week := time.Now().Add(-8 * 24 * time.Hour)
	if err := os.Chtimes(oldBackup, week, week); err != nil {
		t.Fatal(err)
	}

	backups, _ := Backups()
	if len(backups) != 2 || backups[0].Path != newBackup {
		t.Fatalf("want newest first, got %+v", backups)
	}

	old := SelectBackups(backups, 7*24*time.Hour, time.Now())
	if len(old) != 1 || old[0].Path != oldBackup {
		t.Fatalf("only the 8-day-old backup is older than a week: %+v", old)
	}
	if got := SelectBackups(backups, 0, time.Now()); len(got) != 2 {
		t.Fatalf("zero age selects everything, got %d", len(got))
	}

	if n, err := RemoveBackups(old); err != nil || n != 1 {
		t.Fatalf("RemoveBackups = %d, %v", n, err)
	}
	if _, err := os.Stat(oldBackup); !os.IsNotExist(err) {
		t.Fatal("old backup should be gone")
	}
	if _, err := os.Stat(newBackup); err != nil {
		t.Fatal("recent backup should remain")
	}
}

func TestParseAgeAndNames(t *testing.T) {
	for text, want := range map[string]time.Duration{"12h": 12 * time.Hour, "30d": 30 * 24 * time.Hour, "2w": 14 * 24 * time.Hour, "90m": 90 * time.Minute} {
		if got, err := ParseAge(text); err != nil || got != want {
			t.Errorf("ParseAge(%q) = %v, %v, want %v", text, got, err, want)
		}
	}
	for _, bad := range []string{"", "soon", "-3d", "3x"} {
		if _, err := ParseAge(bad); err == nil {
			t.Errorf("ParseAge(%q) should fail", bad)
		}
	}

	op, base, ok := parseBackupName(".seer-job-backup-disable-my-file-123456")
	if !ok || op != "disable" || base != "my-file" {
		t.Fatalf("parseBackupName = %q, %q, %t", op, base, ok)
	}
	if _, _, ok := parseBackupName(".seer-job-backup-123456"); ok {
		t.Fatal("old-style names have no recorded source")
	}
}

func TestTargetedRestoreKeepsLaterEdits(t *testing.T) {
	crontab, _, _ := useTempCron(t)

	original := "# header\n0 6 * * * root /usr/bin/keep\n0 7 * * * root /usr/bin/doomed\n0 8 * * * root /usr/bin/tail\n"
	if err := os.WriteFile(crontab, []byte(original), 0o644); err != nil {
		t.Fatal(err)
	}
	backup, err := Remove(listByCommand(t, "/usr/bin/doomed")[0].ID)
	if err != nil {
		t.Fatal(err)
	}

	// Someone else edits the file after the removal, shifting every line.
	edited := "# new first line\n# header\n0 6 * * * root /usr/bin/keep\n0 9 * * * root /usr/bin/added\n0 8 * * * root /usr/bin/tail\n"
	if err := os.WriteFile(crontab, []byte(edited), 0o644); err != nil {
		t.Fatal(err)
	}

	// The backup summary names the removed job.
	backups, _ := Backups()
	if len(backups) != 1 || backups[0].Summary() != "0 7 * * * root /usr/bin/doomed" {
		t.Fatalf("summary wrong: %+v", backups)
	}

	// Targeted restore adds back just that line, after its anchor line.
	id := BackupID(backup)
	if _, err := Restore(id, false); err != nil {
		t.Fatal(err)
	}
	want := "# new first line\n# header\n0 6 * * * root /usr/bin/keep\n0 7 * * * root /usr/bin/doomed\n0 9 * * * root /usr/bin/added\n0 8 * * * root /usr/bin/tail\n"
	if data, _ := os.ReadFile(crontab); string(data) != want {
		t.Fatalf("targeted restore kept the wrong things:\n%s\nwant\n%s", data, want)
	}

	// Whole-file restore, in contrast, reverts the later edits.
	if _, err := Remove(listByCommand(t, "/usr/bin/doomed")[0].ID); err != nil {
		t.Fatal(err)
	}
	if _, err := Restore(id, true); err != nil {
		t.Fatal(err)
	}
	if data, _ := os.ReadFile(crontab); string(data) != original {
		t.Fatalf("whole-file restore should bring back the saved copy:\n%s", data)
	}
}

func TestTargetedRestoreOfDisableAndEnable(t *testing.T) {
	crontab, _, _ := useTempCron(t)

	if err := os.WriteFile(crontab, []byte("0 6 * * * root /usr/bin/a\n0 7 * * * root /usr/bin/b\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	a := listByCommand(t, "/usr/bin/a")[0]
	b := listByCommand(t, "/usr/bin/b")[0]
	disabled, err := Disable(a.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Disable(b.ID); err != nil {
		t.Fatal(err)
	}

	// Undoing the first disable must not touch the second.
	if _, err := Restore(BackupID(disabled), false); err != nil {
		t.Fatal(err)
	}
	if !listByCommand(t, "/usr/bin/a")[0].Enabled || listByCommand(t, "/usr/bin/b")[0].Enabled {
		t.Fatal("only the first job should be enabled again")
	}

	// If the line was edited since, targeted restore refuses and says why.
	data, _ := os.ReadFile(crontab)
	edited := strings.Replace(string(data), "/usr/bin/b", "/usr/bin/changed", 1)
	_ = os.WriteFile(crontab, []byte(edited), 0o644)
	backups, _ := Backups()
	for _, bk := range backups {
		if bk.Summary() == "0 7 * * * root /usr/bin/b" {
			if _, err := Restore(bk.ID, false); err == nil || !strings.Contains(err.Error(), "--whole-file") {
				t.Fatalf("want an error pointing at --whole-file, got %v", err)
			}
		}
	}
}

func TestRestoreRecreatesDeletedFileAndCleanupRemovesSidecar(t *testing.T) {
	_, cronD, _ := useTempCron(t)

	path := filepath.Join(cronD, "app")
	if err := os.WriteFile(path, []byte("0 3 * * * root /opt/app/run\n"), 0o640); err != nil {
		t.Fatal(err)
	}
	backup, err := Remove(listByCommand(t, "/opt/app/run")[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}

	if _, err := Restore(BackupID(backup), false); err != nil {
		t.Fatal(err)
	}
	if data, _ := os.ReadFile(path); string(data) != "0 3 * * * root /opt/app/run\n" {
		t.Fatalf("recreated file wrong: %q", data)
	}
	if info, _ := os.Stat(path); info.Mode().Perm() != 0o640 {
		t.Fatalf("recreated file has mode %v, want 0640", info.Mode().Perm())
	}

	// The sidecar is never listed as a backup of its own, and cleanup removes it.
	backups, _ := Backups()
	if len(backups) != 1 {
		t.Fatalf("want one backup, got %+v", backups)
	}
	if _, err := os.Stat(backup + ".meta"); err != nil {
		t.Fatal("sidecar should exist")
	}
	if _, err := RemoveBackups(backups); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(backup + ".meta"); !os.IsNotExist(err) {
		t.Fatal("cleanup should remove the sidecar")
	}
}

func TestRestoreSummary(t *testing.T) {
	crontab, _, _ := useTempCron(t)

	if err := os.WriteFile(crontab, []byte("0 6 * * * root /usr/bin/a\n0 7 * * * root /usr/bin/b\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	removed, _ := Remove(listByCommand(t, "/usr/bin/b")[0].ID)
	disabled, _ := Disable(listByCommand(t, "/usr/bin/a")[0].ID)

	got, short, err := RestoreSummary(BackupID(removed), false)
	want := "Re-add to " + crontab + ":\n  + 0 7 * * * root /usr/bin/b\n"
	if err != nil || !short || got != want {
		t.Fatalf("remove summary = %q, %t, %v", got, short, err)
	}

	got, short, err = RestoreSummary(BackupID(disabled), false)
	want = "Change in " + crontab + ":\n  - #[seer-disabled] 0 6 * * * root /usr/bin/a\n  + 0 6 * * * root /usr/bin/a\n"
	if err != nil || !short || got != want {
		t.Fatalf("disable summary = %q, %t, %v", got, short, err)
	}

	if _, short, _ := RestoreSummary(BackupID(removed), true); short {
		t.Fatal("whole-file restores should use the full diff")
	}
}
