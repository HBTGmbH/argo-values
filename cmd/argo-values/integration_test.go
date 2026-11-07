package main

import (
	"bytes"
	"context"
	"errors"
	"flag"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

var binaryPath string

func TestMain(m *testing.M) {
	flag.Parse()
	if testing.Short() {
		os.Exit(m.Run())
	}

	dir, err := os.MkdirTemp("", "argo-values-integration-*")
	if err != nil {
		_, _ = os.Stderr.WriteString("failed to create temp dir: " + err.Error() + "\n")
		os.Exit(1)
	}

	binaryPath = filepath.Join(dir, "argo-values")
	build := exec.CommandContext(context.Background(), "go", "build", "-o", binaryPath, ".")
	build.Stdout = os.Stderr
	build.Stderr = os.Stderr
	if err := build.Run(); err != nil {
		_, _ = os.Stderr.WriteString("failed to build binary: " + err.Error() + "\n")
		_ = os.RemoveAll(dir)
		os.Exit(1)
	}

	code := m.Run()
	_ = os.RemoveAll(dir)
	os.Exit(code)
}

// cleanEnv returns the current environment without inherited ARGOCD_* and LOG_*
// variables so that the tests are deterministic.
func cleanEnv() []string {
	env := make([]string, 0, len(os.Environ()))
	for _, entry := range os.Environ() {
		if strings.HasPrefix(entry, "ARGOCD_") || strings.HasPrefix(entry, "LOG_") {
			continue
		}
		env = append(env, entry)
	}
	return env
}

func runBinary(t *testing.T, env []string, args ...string) (stdout, stderr string, exitCode int) {
	t.Helper()

	cmd := exec.CommandContext(t.Context(), binaryPath, args...)
	cmd.Env = env

	var outBuf, errBuf bytes.Buffer
	cmd.Stdout = &outBuf
	cmd.Stderr = &errBuf

	err := cmd.Run()
	if err != nil {
		var exitErr *exec.ExitError
		if !errors.As(err, &exitErr) {
			t.Fatalf("failed to run %q: %v", args, err)
		}
		exitCode = exitErr.ExitCode()
	}

	return outBuf.String(), errBuf.String(), exitCode
}

func TestIntegrationDefaultHelp(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	stdout, stderr, exitCode := runBinary(t, cleanEnv())

	if exitCode != 0 {
		t.Fatalf("exit code = %d, want 0; stderr: %s", exitCode, stderr)
	}
	if !strings.Contains(stdout, "Usage:") {
		t.Errorf("stdout does not contain %q:\n%s", "Usage:", stdout)
	}
	if !strings.Contains(stdout, "Commands:") {
		t.Errorf("stdout does not contain %q:\n%s", "Commands:", stdout)
	}
}

func TestIntegrationHelpFlag(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	stdout, stderr, exitCode := runBinary(t, cleanEnv(), "--help")

	if exitCode != 0 {
		t.Fatalf("exit code = %d, want 0; stderr: %s", exitCode, stderr)
	}
	if !strings.Contains(stdout, "Usage:") {
		t.Errorf("stdout does not contain %q:\n%s", "Usage:", stdout)
	}
}

func TestIntegrationVersionFlag(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	stdout, stderr, exitCode := runBinary(t, cleanEnv(), "--version")

	if exitCode != 0 {
		t.Fatalf("exit code = %d, want 0; stderr: %s", exitCode, stderr)
	}
	if stdout == "" {
		t.Error("stdout is empty, want version information")
	}
}

func TestIntegrationUnknownFlag(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	stdout, stderr, exitCode := runBinary(t, cleanEnv(), "--bogus")

	if exitCode != 80 {
		t.Fatalf("exit code = %d, want 80; stderr: %s", exitCode, stderr)
	}
	if stdout != "" {
		t.Errorf("stdout = %q, want empty", stdout)
	}
	if stderr == "" {
		t.Error("stderr is empty, want error message")
	}
}

func TestIntegrationInitHelpKeepsLongText(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	stdout, stderr, exitCode := runBinary(t, cleanEnv(), "init", "--help")

	if exitCode != 0 {
		t.Fatalf("exit code = %d, want 0; stderr: %s", exitCode, stderr)
	}
	if !strings.Contains(stdout, "${VAR_NAME}") {
		t.Errorf("stdout does not contain %q (help provider text was interpolated):\n%s", "${VAR_NAME}", stdout)
	}
	if !strings.Contains(stdout, "Initializes") {
		t.Errorf("stdout does not contain %q:\n%s", "Initializes", stdout)
	}
}

func TestIntegrationUnknownCommand(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	_, stderr, exitCode := runBinary(t, cleanEnv(), "foo")

	if exitCode != 80 {
		t.Fatalf("exit code = %d, want 80; stderr: %s", exitCode, stderr)
	}
}

func TestIntegrationDiscoverUnsupported(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	env := append(cleanEnv(), "ARGOCD_APP_PARAMETERS=[]")
	stdout, stderr, exitCode := runBinary(t, env, "discover")

	if exitCode != 2 {
		t.Fatalf("exit code = %d, want 2; stderr: %s", exitCode, stderr)
	}
	if stdout != "" {
		t.Errorf("stdout = %q, want empty", stdout)
	}
}

func TestIntegrationDiscoverSupported(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	env := append(cleanEnv(), "ARGOCD_ENV_VALUE_CONFIGS=cm")
	stdout, stderr, exitCode := runBinary(t, env, "discover")

	if exitCode != 0 {
		t.Fatalf("exit code = %d, want 0; stderr: %s", exitCode, stderr)
	}
	if got, want := stdout, "argo-values\n"; got != want {
		t.Errorf("stdout = %q, want %q", got, want)
	}
}
