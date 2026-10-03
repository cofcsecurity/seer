// pkg/job/edit_test.go
package job

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLocateLineByLineNumber(t *testing.T) {
	path := filepath.Join(t.TempDir(), "crontab")
	content := "a\nb\nc\n"
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	lines, index, _, err := locateLine(path, 2, "b")
	if err != nil {
		t.Fatal(err)
	}
	if index != 1 || lines[index] != "b\n" {
		t.Fatalf("got index %d, line %q", index, lines[index])
	}
}

func TestLocateLineFallsBackWhenShifted(t *testing.T) {
	path := filepath.Join(t.TempDir(), "crontab")
	// "b" has moved from line 2 to line 3 since the caller last saw it.
	content := "a\nx\nb\n"
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	lines, index, _, err := locateLine(path, 2, "b")
	if err != nil {
		t.Fatal(err)
	}
	if index != 2 || lines[index] != "b\n" {
		t.Fatalf("got index %d, line %q", index, lines[index])
	}
}

func TestLocateLineAmbiguous(t *testing.T) {
	path := filepath.Join(t.TempDir(), "crontab")
	content := "b\nx\nb\n"
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	if _, _, _, err := locateLine(path, 5, "b"); err == nil || !strings.Contains(err.Error(), "matched 2 lines") {
		t.Fatalf("expected ambiguous match error, got %v", err)
	}
}

func TestLocateLineNotFound(t *testing.T) {
	path := filepath.Join(t.TempDir(), "crontab")
	if err := os.WriteFile(path, []byte("a\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if _, _, _, err := locateLine(path, 5, "missing"); err == nil || !strings.Contains(err.Error(), "was not found") {
		t.Fatalf("expected not-found error, got %v", err)
	}
}

func TestLocateLineRejectsNonRegular(t *testing.T) {
	dir := t.TempDir()
	if _, _, _, err := locateLine(dir, 1, "anything"); err == nil || !strings.Contains(err.Error(), "not a regular file") {
		t.Fatalf("expected non-regular-file error, got %v", err)
	}
}

func TestWriteBackup(t *testing.T) {
	dir := t.TempDir()
	data := []byte("original content\n")
	backupPath, err := writeBackup(dir, data)
	if err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(backupPath)
	if err != nil || string(got) != string(data) {
		t.Fatalf("backup content = %q, %v", got, err)
	}
}

func TestCommitReplacementDetectsChange(t *testing.T) {
	path := filepath.Join(t.TempDir(), "crontab")
	original := []byte("a\nb\n")
	if err := os.WriteFile(path, original, 0644); err != nil {
		t.Fatal(err)
	}
	info, err := os.Lstat(path)
	if err != nil {
		t.Fatal(err)
	}
	// Mutate the file after info/original were captured.
	if err := os.WriteFile(path, []byte("a\nchanged\n"), 0644); err != nil {
		t.Fatal(err)
	}
	tmpPath := filepath.Join(t.TempDir(), "tmp")
	if err := os.WriteFile(tmpPath, []byte("new\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := commitReplacement(path, info, original, tmpPath); err == nil || !strings.Contains(err.Error(), "changed") {
		t.Fatalf("expected changed error, got %v", err)
	}
}

func TestCommitReplacementSucceeds(t *testing.T) {
	path := filepath.Join(t.TempDir(), "crontab")
	original := []byte("a\nb\n")
	if err := os.WriteFile(path, original, 0644); err != nil {
		t.Fatal(err)
	}
	info, err := os.Lstat(path)
	if err != nil {
		t.Fatal(err)
	}
	tmpPath := filepath.Join(t.TempDir(), "tmp")
	if err := os.WriteFile(tmpPath, []byte("new\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := commitReplacement(path, info, original, tmpPath); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(path)
	if err != nil || string(got) != "new\n" {
		t.Fatalf("got %q, %v", got, err)
	}
}

func TestRewriteSourceDisableAndRemove(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "crontab")
	original := "# keep this comment\n* * * * * root /usr/bin/true\n0 0 * * * root /usr/bin/false\n"
	if err := os.WriteFile(path, []byte(original), 0600); err != nil {
		t.Fatal(err)
	}

	target := Job{Source: path, LineNumber: 2, Raw: "* * * * * root /usr/bin/true"}
	replacement := "# * * * * * root /usr/bin/true"
	backup, err := rewriteSource(target, &replacement)
	if err != nil {
		t.Fatal(err)
	}

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	want := "# keep this comment\n# * * * * * root /usr/bin/true\n0 0 * * * root /usr/bin/false\n"
	if string(got) != want {
		t.Fatalf("after disable, got %q, want %q", got, want)
	}

	old, err := os.ReadFile(backup)
	if err != nil || string(old) != original {
		t.Fatalf("backup differs: %q, %v", old, err)
	}

	info, err := os.Stat(path)
	if err != nil || info.Mode().Perm() != 0600 {
		t.Fatalf("permissions changed: %v, %v", info, err)
	}

	// Now remove the second job entirely.
	target2 := Job{Source: path, LineNumber: 3, Raw: "0 0 * * * root /usr/bin/false"}
	if _, err := rewriteSource(target2, nil); err != nil {
		t.Fatal(err)
	}
	got, err = os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	want = "# keep this comment\n# * * * * * root /usr/bin/true\n"
	if string(got) != want {
		t.Fatalf("after remove, got %q, want %q", got, want)
	}
}

func TestRewriteSourceStaleJobFails(t *testing.T) {
	path := filepath.Join(t.TempDir(), "crontab")
	if err := os.WriteFile(path, []byte("* * * * * root /usr/bin/true\n"), 0600); err != nil {
		t.Fatal(err)
	}
	target := Job{Source: path, LineNumber: 1, Raw: "* * * * * root /usr/bin/gone"}
	if _, err := rewriteSource(target, nil); err == nil {
		t.Fatal("expected error for a job that no longer matches")
	}
	got, err := os.ReadFile(path)
	if err != nil || string(got) != "* * * * * root /usr/bin/true\n" {
		t.Fatalf("file should be untouched, got %q, %v", got, err)
	}
}
