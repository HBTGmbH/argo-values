package commands

import (
	"os"
	"path/filepath"
	"testing"

	"argo-values/internal/helm"
)

func writeTestFile(t *testing.T, path, content string) {
	t.Helper()

	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatalf("failed to create directory for %s: %v", path, err)
	}
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write %s: %v", path, err)
	}
}

func readTestFile(t *testing.T, path string) string {
	t.Helper()

	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed to read %s: %v", path, err)
	}
	return string(content)
}

func TestPrepareApplicationFilesNestedDirectories(t *testing.T) {
	root := t.TempDir()
	files := []string{
		"a.txt",
		"sub/b.txt",
		"sub/deep/c.txt",
		"sub/deep/deeper/d.txt",
	}
	for _, rel := range files {
		writeTestFile(t, filepath.Join(root, rel), "${A}")
	}

	env := map[string]string{"A": "done"}
	if err := prepareApplicationFiles(root, &helm.IgnoreFile{}, env, false); err != nil {
		t.Fatalf("prepareApplicationFiles returned error: %v", err)
	}

	for _, rel := range files {
		if got := readTestFile(t, filepath.Join(root, rel)); got != "done" {
			t.Errorf("%s = %q, want %q", rel, got, "done")
		}
	}
}

func TestPrepareApplicationFilesProcessesEachFileExactlyOnce(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "file.txt")
	writeTestFile(t, path, "${A}")

	env := map[string]string{
		"A": "${B}",
		"B": "done",
	}
	if err := prepareApplicationFiles(root, &helm.IgnoreFile{}, env, false); err != nil {
		t.Fatalf("prepareApplicationFiles returned error: %v", err)
	}

	if got := readTestFile(t, path); got != "${B}" {
		t.Errorf("file content = %q, want %q (file must be processed exactly once)", got, "${B}")
	}
}

func TestPrepareApplicationFilesRespectsHelmIgnore(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, filepath.Join(root, ".helmignore"), "ignored.txt\n")
	ignoredPath := filepath.Join(root, "ignored.txt")
	keptPath := filepath.Join(root, "kept.txt")
	writeTestFile(t, ignoredPath, "${A}")
	writeTestFile(t, keptPath, "${A}")

	ignorePatterns, err := helm.ReadHelmIgnore(root)
	if err != nil {
		t.Fatalf("failed to read .helmignore: %v", err)
	}

	env := map[string]string{"A": "done"}
	if err := prepareApplicationFiles(root, ignorePatterns, env, false); err != nil {
		t.Fatalf("prepareApplicationFiles returned error: %v", err)
	}

	if got := readTestFile(t, ignoredPath); got != "${A}" {
		t.Errorf("ignored file content = %q, want %q", got, "${A}")
	}
	if got := readTestFile(t, keptPath); got != "done" {
		t.Errorf("kept file content = %q, want %q", got, "done")
	}
}

func TestPrepareApplicationFilesDryRunLeavesFilesUnchanged(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "file.txt")
	writeTestFile(t, path, "${A}")

	env := map[string]string{"A": "done"}
	if err := prepareApplicationFiles(root, &helm.IgnoreFile{}, env, true); err != nil {
		t.Fatalf("prepareApplicationFiles returned error: %v", err)
	}

	if got := readTestFile(t, path); got != "${A}" {
		t.Errorf("file content = %q, want unchanged %q", got, "${A}")
	}
}

func TestProcessFile(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "file.txt")
	writeTestFile(t, path, "${KNOWN} ${UNKNOWN} ${db.host}")

	env := map[string]string{
		"KNOWN":   "yes",
		"DB_HOST": "db.example.com",
	}
	if err := processFile(root, path, env, false); err != nil {
		t.Fatalf("processFile returned error: %v", err)
	}

	want := "yes ${UNKNOWN} db.example.com"
	if got := readTestFile(t, path); got != want {
		t.Errorf("file content = %q, want %q", got, want)
	}
}
