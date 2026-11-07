package main

import (
	"bytes"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"argo-values/internal/config"
	"argo-values/internal/helm"
	"argo-values/internal/logger"

	"github.com/sirupsen/logrus"
	v1 "k8s.io/api/core/v1"
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

func TestMergeDataIsDeterministic(t *testing.T) {
	configMaps := map[string]*v1.ConfigMap{
		"cm": {Data: map[string]string{
			"a": "key: first",
			"b": "key: second",
		}},
	}

	var first map[string]any
	for i := range 100 {
		result, err := mergeData(configMaps, []string{"cm"}, nil, nil, config.ParseYAML)
		if err != nil {
			t.Fatalf("mergeData() returned error on iteration %d: %v", i, err)
		}
		if i == 0 {
			first = result
		} else if !reflect.DeepEqual(result, first) {
			t.Fatalf("mergeData() returned a different result on iteration %d: got %#v, want %#v", i, result, first)
		}

		if key, ok := result["key"]; !ok || key != "second" {
			t.Fatalf("mergeData()[key] = %#v, want %q (sorted key %q must win)", key, "second", "b")
		}
	}
}

func TestMergeDataSourcePrecedence(t *testing.T) {
	cm1 := &v1.ConfigMap{Data: map[string]string{"values": "key: first"}}
	cm2 := &v1.ConfigMap{Data: map[string]string{"values": "key: second"}}
	secret := &v1.Secret{Data: map[string][]byte{"values": []byte("key: secret")}}

	configMaps := map[string]*v1.ConfigMap{"cm1": cm1, "cm2": cm2}
	secrets := map[string]*v1.Secret{"secret": secret}

	got, err := mergeData(configMaps, []string{"cm1", "cm2"}, nil, nil, config.ParseYAML)
	if err != nil {
		t.Fatalf("mergeData() returned error: %v", err)
	}
	if got["key"] != "second" {
		t.Errorf("later ConfigMap must win: key = %#v, want %q", got["key"], "second")
	}

	got, err = mergeData(configMaps, []string{"cm1", "cm2"}, secrets, []string{"secret"}, config.ParseYAML)
	if err != nil {
		t.Fatalf("mergeData() returned error: %v", err)
	}
	if got["key"] != "secret" {
		t.Errorf("Secret after ConfigMaps must win: key = %#v, want %q", got["key"], "secret")
	}
}

func TestMergeDataEnvModePreservesKeys(t *testing.T) {
	configMaps := map[string]*v1.ConfigMap{
		"cm": {Data: map[string]string{
			"CONFIG_JSON": `{"a":1}`,
			"EMPTY":       "",
			"NULL_VALUE":  "null",
		}},
	}

	merged, err := mergeData(configMaps, []string{"cm"}, nil, nil, config.ParseEnv)
	if err != nil {
		t.Fatalf("mergeData() returned error: %v", err)
	}
	flat := map[string]string{}
	config.FlattenYAML(merged, "", flat)

	if got := flat["CONFIG_JSON"]; got != `{"a":1}` {
		t.Errorf("flat[CONFIG_JSON] = %q, want %q", got, `{"a":1}`)
	}
	if got := flat["EMPTY"]; got != "" {
		t.Errorf("flat[EMPTY] = %q, want %q", got, "")
	}
	if got := flat["NULL_VALUE"]; got != "null" {
		t.Errorf("flat[NULL_VALUE] = %q, want %q", got, "null")
	}
}

func TestMergeDataValuesModeParsesYAML(t *testing.T) {
	configMaps := map[string]*v1.ConfigMap{
		"cm": {Data: map[string]string{
			"values.yaml": "resources:\n  requests:\n    cpu: \"123m\"\n",
		}},
	}

	merged, err := mergeData(configMaps, []string{"cm"}, nil, nil, config.ParseYAML)
	if err != nil {
		t.Fatalf("mergeData() returned error: %v", err)
	}

	expected := map[string]any{
		"resources": map[string]any{
			"requests": map[string]any{
				"cpu": "123m",
			},
		},
	}
	if !reflect.DeepEqual(merged, expected) {
		t.Errorf("merged = %#v, want %#v", merged, expected)
	}
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

func TestPrepareApplicationFilesHelmIgnorePathPattern(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, filepath.Join(root, ".helmignore"), "templates/*.yaml\n")
	files := []string{
		"templates/a.yaml",
		"templates/sub/b.yaml",
		"templates/sub/deep/c.yaml",
	}
	for _, rel := range files {
		writeTestFile(t, filepath.Join(root, rel), "${A}")
	}

	ignorePatterns, err := helm.ReadHelmIgnore(root)
	if err != nil {
		t.Fatalf("failed to read .helmignore: %v", err)
	}

	env := map[string]string{"A": "done"}
	if err := prepareApplicationFiles(root, ignorePatterns, env, false); err != nil {
		t.Fatalf("prepareApplicationFiles returned error: %v", err)
	}

	if got := readTestFile(t, filepath.Join(root, "templates/a.yaml")); got != "${A}" {
		t.Errorf("templates/a.yaml = %q, want unchanged %q", got, "${A}")
	}
	for _, rel := range files[1:] {
		if got := readTestFile(t, filepath.Join(root, rel)); got != "done" {
			t.Errorf("%s = %q, want %q", rel, got, "done")
		}
	}
}

func TestPrepareApplicationFilesHelmIgnoreDirectoryPattern(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, filepath.Join(root, ".helmignore"), "foo/\n")
	files := []string{
		"foo/a.txt",
		"sub/foo/b.txt",
	}
	for _, rel := range files {
		writeTestFile(t, filepath.Join(root, rel), "${A}")
	}

	ignorePatterns, err := helm.ReadHelmIgnore(root)
	if err != nil {
		t.Fatalf("failed to read .helmignore: %v", err)
	}

	env := map[string]string{"A": "done"}
	if err := prepareApplicationFiles(root, ignorePatterns, env, false); err != nil {
		t.Fatalf("prepareApplicationFiles returned error: %v", err)
	}

	for _, rel := range files {
		if got := readTestFile(t, filepath.Join(root, rel)); got != "${A}" {
			t.Errorf("%s = %q, want unchanged %q (directory must be skipped)", rel, got, "${A}")
		}
	}
}

func TestPrepareApplicationFilesHelmIgnoreBasenamePattern(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, filepath.Join(root, ".helmignore"), "foo.txt\n")
	files := []string{
		"foo.txt",
		"sub/foo.txt",
	}
	for _, rel := range files {
		writeTestFile(t, filepath.Join(root, rel), "${A}")
	}

	ignorePatterns, err := helm.ReadHelmIgnore(root)
	if err != nil {
		t.Fatalf("failed to read .helmignore: %v", err)
	}

	env := map[string]string{"A": "done"}
	if err := prepareApplicationFiles(root, ignorePatterns, env, false); err != nil {
		t.Fatalf("prepareApplicationFiles returned error: %v", err)
	}

	for _, rel := range files {
		if got := readTestFile(t, filepath.Join(root, rel)); got != "${A}" {
			t.Errorf("%s = %q, want unchanged %q", rel, got, "${A}")
		}
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

func TestProcessFileDoesNotRewriteUnchangedFile(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "file.txt")
	writeTestFile(t, path, "no placeholders here")

	oldTime := time.Now().Add(-time.Hour).Truncate(time.Second)
	if err := os.Chtimes(path, oldTime, oldTime); err != nil {
		t.Fatalf("failed to set mtime: %v", err)
	}

	env := map[string]string{"A": "done"}
	if err := processFile(root, path, env, false); err != nil {
		t.Fatalf("processFile returned error: %v", err)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("failed to stat file: %v", err)
	}
	if !info.ModTime().Equal(oldTime) {
		t.Errorf("mtime = %v, want unchanged %v (file must not be rewritten)", info.ModTime(), oldTime)
	}
}

func TestProcessFileSkipsBinaryFile(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "binary.bin")
	content := "prefix\x00${A}suffix"
	writeTestFile(t, path, content)

	env := map[string]string{"A": "done"}
	if err := processFile(root, path, env, false); err != nil {
		t.Fatalf("processFile returned error: %v", err)
	}

	if got := readTestFile(t, path); got != content {
		t.Errorf("file content = %q, want unchanged %q", got, content)
	}
}

func TestMergeDataRejectsMissingConfigMap(t *testing.T) {
	_, err := mergeData(
		map[string]*v1.ConfigMap{},
		[]string{"missing"},
		map[string]*v1.Secret{},
		nil,
		config.ParseYAML,
	)
	if err == nil {
		t.Fatal("mergeData() returned nil error for missing ConfigMap, want error")
	}
	if !strings.Contains(err.Error(), "missing") {
		t.Errorf("error = %q, want it to mention %q", err.Error(), "missing")
	}
}

func TestMergeDataRejectsMissingSecret(t *testing.T) {
	_, err := mergeData(
		map[string]*v1.ConfigMap{},
		nil,
		map[string]*v1.Secret{},
		[]string{"missing"},
		config.ParseYAML,
	)
	if err == nil {
		t.Fatal("mergeData() returned nil error for missing Secret, want error")
	}
	if !strings.Contains(err.Error(), "missing") {
		t.Errorf("error = %q, want it to mention %q", err.Error(), "missing")
	}
}
