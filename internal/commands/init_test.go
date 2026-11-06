package commands

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"argo-values/internal/helm"
	"argo-values/internal/logger"

	"github.com/sirupsen/logrus"
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

func TestPrepareApplicationFilesSkipsSymlink(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	target := filepath.Join(outside, "target.txt")
	writeTestFile(t, target, "${A}")

	link := filepath.Join(root, "link.txt")
	if err := os.Symlink(target, link); err != nil {
		t.Fatalf("failed to create symlink: %v", err)
	}

	env := map[string]string{"A": "done"}
	if err := prepareApplicationFiles(root, &helm.IgnoreFile{}, env, false); err != nil {
		t.Fatalf("prepareApplicationFiles returned error: %v", err)
	}

	if got := readTestFile(t, target); got != "${A}" {
		t.Errorf("target content = %q, want unchanged %q", got, "${A}")
	}
	info, err := os.Lstat(link)
	if err != nil {
		t.Fatalf("failed to lstat symlink: %v", err)
	}
	if info.Mode()&os.ModeSymlink == 0 {
		t.Errorf("link is no longer a symlink, mode = %v", info.Mode())
	}
}

func TestProcessFileRefusesSymlink(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	target := filepath.Join(outside, "target.txt")
	writeTestFile(t, target, "${A}")

	link := filepath.Join(root, "link.txt")
	if err := os.Symlink(target, link); err != nil {
		t.Fatalf("failed to create symlink: %v", err)
	}

	env := map[string]string{"A": "done"}
	if err := processFile(root, link, env, false); err == nil {
		t.Fatal("processFile returned nil error for symlink, want error")
	}

	if got := readTestFile(t, target); got != "${A}" {
		t.Errorf("target content = %q, want unchanged %q", got, "${A}")
	}
}

func TestProcessFileDoesNotLogSecretValue(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "file.txt")
	writeTestFile(t, path, "${SECRET}")

	// Redirect logger output and restore the global logger state afterwards.
	originalOut := logger.Log.Out
	originalLevel := logger.Log.GetLevel()
	originalFormatter := logger.Log.Formatter
	defer func() {
		logger.Log.SetOutput(originalOut)
		logger.Log.SetLevel(originalLevel)
		logger.Log.SetFormatter(originalFormatter)
	}()

	var buf bytes.Buffer
	logger.Log.SetOutput(&buf)
	logger.Log.SetLevel(logrus.DebugLevel)
	logger.Log.SetFormatter(&logrus.TextFormatter{DisableTimestamp: true})

	const secretValue = "super-secret-value"
	env := map[string]string{"SECRET": secretValue}
	if err := processFile(root, path, env, false); err != nil {
		t.Fatalf("processFile returned error: %v", err)
	}

	if got := readTestFile(t, path); got != secretValue {
		t.Errorf("file content = %q, want %q (substitution must still work)", got, secretValue)
	}

	logOutput := buf.String()
	if strings.Contains(logOutput, secretValue) {
		t.Errorf("log output must not contain the secret value %q, got: %s", secretValue, logOutput)
	}
	if !strings.Contains(logOutput, "SECRET") {
		t.Errorf("log output must contain the key %q, got: %s", "SECRET", logOutput)
	}
}
