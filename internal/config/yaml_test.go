package config

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestParseYAML(t *testing.T) {
	tests := []struct {
		name     string
		key      string
		value    string
		expected map[string]any
	}{
		{
			name:     "valid map",
			key:      "test",
			value:    "key: value",
			expected: map[string]any{"key": "value"},
		},
		{
			name:     "scalar quantity stays string",
			key:      "key",
			value:    "123m",
			expected: map[string]any{"key": "123m"},
		},
		{
			name:     "empty value keeps key",
			key:      "key",
			value:    "",
			expected: map[string]any{"key": ""},
		},
		{
			name:     "null value stays literal",
			key:      "key",
			value:    "null",
			expected: map[string]any{"key": "null"},
		},
		{
			name:     "broken structured yaml falls back",
			key:      "key",
			value:    "resources:\n  cpu: [",
			expected: map[string]any{"key": "resources:\n  cpu: ["},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ParseYAML(tt.key, tt.value)
			if !reflect.DeepEqual(result, tt.expected) {
				t.Errorf("ParseYAML(%q, %q) = %#v, want %#v", tt.key, tt.value, result, tt.expected)
			}
		})
	}
}

func TestParseEnv(t *testing.T) {
	tests := []struct {
		name     string
		key      string
		value    string
		expected map[string]any
	}{
		{
			name:     "json object stays literal",
			key:      "CONFIG_JSON",
			value:    `{"a":1}`,
			expected: map[string]any{"CONFIG_JSON": `{"a":1}`},
		},
		{
			name:     "empty value stays literal",
			key:      "EMPTY",
			value:    "",
			expected: map[string]any{"EMPTY": ""},
		},
		{
			name:     "null stays literal",
			key:      "NULL_VALUE",
			value:    "null",
			expected: map[string]any{"NULL_VALUE": "null"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ParseEnv(tt.key, tt.value)
			if !reflect.DeepEqual(result, tt.expected) {
				t.Errorf("ParseEnv(%q, %q) = %#v, want %#v", tt.key, tt.value, result, tt.expected)
			}
		})
	}
}

func TestSanitizeKey(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "dots and dashes",
			input:    "some.key-name",
			expected: "SOME_KEY_NAME",
		},
		{
			name:     "only dots",
			input:    "some.key.with.dots",
			expected: "SOME_KEY_WITH_DOTS",
		},
		{
			name:     "only dashes",
			input:    "some-key-with-dashes",
			expected: "SOME_KEY_WITH_DASHES",
		},
		{
			name:     "no special chars",
			input:    "simplekey",
			expected: "SIMPLEKEY",
		},
		{
			name:     "empty string",
			input:    "",
			expected: "",
		},
		{
			name:     "mixed case",
			input:    "Some.Mixed-Case",
			expected: "SOME_MIXED_CASE",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := SanitizeKey(tt.input)
			if result != tt.expected {
				t.Errorf("SanitizeKey() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestFlattenYAML(t *testing.T) {
	tests := []struct {
		name     string
		input    map[string]any
		expected map[string]string
	}{
		{
			name:     "flat structure",
			input:    map[string]any{"key1": "value1", "key2": "value2"},
			expected: map[string]string{"KEY1": "value1", "KEY2": "value2"},
		},
		{
			name:     "nested structure",
			input:    map[string]any{"outer": map[string]any{"inner": "value"}},
			expected: map[string]string{"OUTER_INNER": "value"},
		},
		{
			name:     "deeply nested",
			input:    map[string]any{"a": map[string]any{"b": map[string]any{"c": "deep"}}},
			expected: map[string]string{"A_B_C": "deep"},
		},
		{
			name:     "mixed types",
			input:    map[string]any{"string": "text", "number": 42, "bool": true},
			expected: map[string]string{"STRING": "text", "NUMBER": "42", "BOOL": "true"},
		},
		{
			name:     "empty map",
			input:    map[string]any{},
			expected: map[string]string{},
		},
		{
			name: "complex nested",
			input: map[string]any{
				"database": map[string]any{
					"host": "localhost",
					"port": 5432,
					"credentials": map[string]any{
						"username": "admin",
						"password": "secret",
					},
				},
			},
			expected: map[string]string{
				"DATABASE_HOST":                 "localhost",
				"DATABASE_PORT":                 "5432",
				"DATABASE_CREDENTIALS_USERNAME": "admin",
				"DATABASE_CREDENTIALS_PASSWORD": "secret",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := make(map[string]string)
			FlattenYAML(tt.input, "", result)

			if len(result) != len(tt.expected) {
				t.Errorf("FlattenYAML() length = %d, want %d", len(result), len(tt.expected))
				return
			}

			for k, expectedVal := range tt.expected {
				if resultVal, exists := result[k]; !exists {
					t.Errorf("FlattenYAML() missing key %s", k)
				} else if resultVal != expectedVal {
					t.Errorf("FlattenYAML()[%s] = %v, want %v", k, resultVal, expectedVal)
				}
			}
		})
	}
}

func TestMergeYAML(t *testing.T) {
	tests := []struct {
		name     string
		a        map[string]any
		b        map[string]any
		expected map[string]any
	}{
		{
			name:     "simple merge",
			a:        map[string]any{"key1": "value1"},
			b:        map[string]any{"key2": "value2"},
			expected: map[string]any{"key1": "value1", "key2": "value2"},
		},
		{
			name:     "overwrite value",
			a:        map[string]any{"key": "old"},
			b:        map[string]any{"key": "new"},
			expected: map[string]any{"key": "new"},
		},
		{
			name:     "merge nested maps",
			a:        map[string]any{"outer": map[string]any{"inner1": "value1"}},
			b:        map[string]any{"outer": map[string]any{"inner2": "value2"}},
			expected: map[string]any{"outer": map[string]any{"inner1": "value1", "inner2": "value2"}},
		},
		{
			name:     "deep merge",
			a:        map[string]any{"a": map[string]any{"b": map[string]any{"c": "old"}}},
			b:        map[string]any{"a": map[string]any{"b": map[string]any{"d": "new"}}},
			expected: map[string]any{"a": map[string]any{"b": map[string]any{"c": "old", "d": "new"}}},
		},
		{
			name:     "empty maps",
			a:        map[string]any{},
			b:        map[string]any{"key": "value"},
			expected: map[string]any{"key": "value"},
		},
		{
			name:     "overwrite nested with simple",
			a:        map[string]any{"key": map[string]any{"nested": "value"}},
			b:        map[string]any{"key": "simple"},
			expected: map[string]any{"key": "simple"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := MergeYAML(tt.a, tt.b)
			compareMaps(t, result, tt.expected)
		})
	}
}

func TestCoerceScalars(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  any
	}{
		{name: "bool true lowercase", input: "true", want: true},
		{name: "bool true uppercase", input: "True", want: true},
		{name: "bool false lowercase", input: "false", want: false},
		{name: "bool false uppercase", input: "FALSE", want: false},
		{name: "null lowercase", input: "null", want: nil},
		{name: "null uppercase", input: "NULL", want: nil},
		{name: "null tilde", input: "~", want: nil},
		{name: "int", input: "3", want: int64(3)},
		{name: "negative int", input: "-3", want: int64(-3)},
		{name: "zero", input: "0", want: int64(0)},
		{name: "explicitly positive int", input: "+3", want: int64(3)},
		{name: "float", input: "0.5", want: float64(0.5)},
		{name: "negative float", input: "-0.5", want: float64(-0.5)},
		{name: "exponent", input: "1e3", want: float64(1e3)},
		{name: "exponent with fraction", input: "1.5e-3", want: float64(1.5e-3)},
		{name: "quantity", input: "123m", want: "123m"},
		{name: "leading zero", input: "0123", want: "0123"},
		{name: "negative leading zero", input: "-0123", want: "-0123"},
		{name: "underscore number", input: "1_000", want: "1_000"},
		{name: "hex number", input: "0x10", want: "0x10"},
		{name: "timestamp", input: "2024-01-01", want: "2024-01-01"},
		{name: "comment", input: "#c", want: "#c"},
		{name: "flow sequence", input: "[1,2]", want: "[1,2]"},
		{name: "inline map", input: "a: b", want: "a: b"},
		{name: "empty string", input: "", want: ""},
		{name: "quoted string", input: "'false'", want: "'false'"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := CoerceScalars(map[string]any{"value": tt.input})["value"]
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("CoerceScalars(%q) = %#v (%T), want %#v (%T)", tt.input, got, got, tt.want, tt.want)
			}
		})
	}
}

func TestCoerceScalarsNested(t *testing.T) {
	input := map[string]any{
		"outer": map[string]any{
			"replicas": "3",
			"enabled":  "false",
			"list":     []any{"true", "0.5", "123m", "~"},
		},
	}
	want := map[string]any{
		"outer": map[string]any{
			"replicas": int64(3),
			"enabled":  false,
			"list":     []any{true, float64(0.5), "123m", nil},
		},
	}

	if got := CoerceScalars(input); !reflect.DeepEqual(got, want) {
		t.Errorf("CoerceScalars() = %#v, want %#v", got, want)
	}
}

func TestCoerceScalarsDoesNotMutateInput(t *testing.T) {
	input := map[string]any{
		"replicas": "3",
		"enabled":  "false",
		"nested": map[string]any{
			"ratio": "0.5",
		},
		"list": []any{"true", "1"},
	}
	original := map[string]any{
		"replicas": "3",
		"enabled":  "false",
		"nested": map[string]any{
			"ratio": "0.5",
		},
		"list": []any{"true", "1"},
	}

	CoerceScalars(input)

	if !reflect.DeepEqual(input, original) {
		t.Errorf("CoerceScalars() mutated input: got %#v, want %#v", input, original)
	}
}

func TestUnflattenSanitizedYAML(t *testing.T) {
	tests := []struct {
		name     string
		input    map[string]string
		expected map[string]any
	}{
		{
			name:     "single flat key",
			input:    map[string]string{"REPLICAS": "3"},
			expected: map[string]any{"replicas": "3"},
		},
		{
			name:     "nested key",
			input:    map[string]string{"RESOURCES_REQUESTS_CPU": "123m"},
			expected: map[string]any{"resources": map[string]any{"requests": map[string]any{"cpu": "123m"}}},
		},
		{
			name: "multiple nested keys",
			input: map[string]string{
				"RESOURCES_REQUESTS_CPU":    "123m",
				"RESOURCES_REQUESTS_MEMORY": "256Mi",
			},
			expected: map[string]any{
				"resources": map[string]any{
					"requests": map[string]any{
						"cpu":    "123m",
						"memory": "256Mi",
					},
				},
			},
		},
		{
			name: "mixed depth",
			input: map[string]string{
				"REPLICAS":                  "3",
				"RESOURCES_REQUESTS_CPU":    "123m",
				"RESOURCES_REQUESTS_MEMORY": "256Mi",
			},
			expected: map[string]any{
				"replicas": "3",
				"resources": map[string]any{
					"requests": map[string]any{
						"cpu":    "123m",
						"memory": "256Mi",
					},
				},
			},
		},
		{
			name:     "empty map",
			input:    map[string]string{},
			expected: map[string]any{},
		},
		{
			name: "complex nested",
			input: map[string]string{
				"DATABASE_HOST":                 "localhost",
				"DATABASE_PORT":                 "5432",
				"DATABASE_CREDENTIALS_USERNAME": "admin",
				"DATABASE_CREDENTIALS_PASSWORD": "secret",
			},
			expected: map[string]any{
				"database": map[string]any{
					"host": "localhost",
					"port": "5432",
					"credentials": map[string]any{
						"username": "admin",
						"password": "secret",
					},
				},
			},
		},
		{
			name: "conflict leaf overwritten by map",
			input: map[string]string{
				"A":   "leaf",
				"A_B": "nested",
			},
			expected: map[string]any{
				"a": map[string]any{
					"b": "nested",
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := UnflattenYAML(tt.input, true)

			if len(result) != len(tt.expected) {
				t.Errorf("UnflattenYAML() length = %d, want %d", len(result), len(tt.expected))
				return
			}

			compareMaps(t, result, tt.expected)
		})
	}
}

func TestUnflattenUnSanitizedYAML(t *testing.T) {
	tests := []struct {
		name     string
		input    map[string]string
		expected map[string]any
	}{
		{
			name:     "single flat key",
			input:    map[string]string{"replicas": "3"},
			expected: map[string]any{"replicas": "3"},
		},
		{
			name:     "nested key",
			input:    map[string]string{"resources.requests.cpu": "123m"},
			expected: map[string]any{"resources": map[string]any{"requests": map[string]any{"cpu": "123m"}}},
		},
		{
			name: "multiple nested keys",
			input: map[string]string{
				"resources.requests.cpu":    "123m",
				"resources.requests.memory": "256Mi",
			},
			expected: map[string]any{
				"resources": map[string]any{
					"requests": map[string]any{
						"cpu":    "123m",
						"memory": "256Mi",
					},
				},
			},
		},
		{
			name: "mixed depth",
			input: map[string]string{
				"replicas":                  "3",
				"resources.requests.cpu":    "123m",
				"resources.requests.memory": "256Mi",
			},
			expected: map[string]any{
				"replicas": "3",
				"resources": map[string]any{
					"requests": map[string]any{
						"cpu":    "123m",
						"memory": "256Mi",
					},
				},
			},
		},
		{
			name:     "empty map",
			input:    map[string]string{},
			expected: map[string]any{},
		},
		{
			name: "complex nested",
			input: map[string]string{
				"database.host":                 "localhost",
				"database.port":                 "5432",
				"database.credentials.username": "admin",
				"database.credentials.password": "secret",
			},
			expected: map[string]any{
				"database": map[string]any{
					"host": "localhost",
					"port": "5432",
					"credentials": map[string]any{
						"username": "admin",
						"password": "secret",
					},
				},
			},
		},
		{
			name: "conflict leaf overwritten by map",
			input: map[string]string{
				"a":   "leaf",
				"a.b": "nested",
			},
			expected: map[string]any{
				"a": map[string]any{
					"b": "nested",
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := UnflattenYAML(tt.input, false)

			if len(result) != len(tt.expected) {
				t.Errorf("UnflattenYAML() length = %d, want %d", len(result), len(tt.expected))
				return
			}

			compareMaps(t, result, tt.expected)
		})
	}
}

func TestFlattenUnflattenRoundTrip(t *testing.T) {
	input := map[string]any{
		"resources": map[string]any{
			"requests": map[string]any{
				"cpu":    "123m",
				"memory": "256Mi",
			},
		},
		"replicas": "3",
	}

	flat := make(map[string]string)
	FlattenYAML(input, "", flat)
	result := UnflattenYAML(flat, true)

	compareMaps(t, result, input)
}

func TestUnflattenYAMLDeterministic(t *testing.T) {
	tests := []struct {
		name      string
		input     map[string]string
		sanitized bool
		expected  map[string]any
	}{
		{
			name:      "conflict leaf overwritten by map unsanitized",
			input:     map[string]string{"a": "leaf", "a.b": "nested"},
			sanitized: false,
			expected: map[string]any{
				"a": map[string]any{"b": "nested"},
			},
		},
		{
			name:      "conflict leaf overwritten by map sanitized",
			input:     map[string]string{"A": "leaf", "A_B": "nested"},
			sanitized: true,
			expected: map[string]any{
				"a": map[string]any{"b": "nested"},
			},
		},
		{
			name:      "deeper path overwrites intermediate leaf",
			input:     map[string]string{"a.b": "leaf", "a.b.c": "deep"},
			sanitized: false,
			expected: map[string]any{
				"a": map[string]any{
					"b": map[string]any{"c": "deep"},
				},
			},
		},
		{
			name: "multiple conflicts in different branches",
			input: map[string]string{
				"x":     "leaf",
				"x.y":   "nested",
				"p.q":   "leaf",
				"p.q.r": "deep",
			},
			sanitized: false,
			expected: map[string]any{
				"x": map[string]any{"y": "nested"},
				"p": map[string]any{
					"q": map[string]any{"r": "deep"},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for i := range 1000 {
				result := UnflattenYAML(tt.input, tt.sanitized)
				compareMaps(t, result, tt.expected)
				if t.Failed() {
					t.Fatalf("non-deterministic result on iteration %d", i)
				}
			}
		})
	}
}

func TestUnflattenYAMLArrays(t *testing.T) {
	tests := []struct {
		name     string
		input    map[string]string
		expected map[string]any
	}{
		{
			name:  "scalar list",
			input: map[string]string{"env[0]": "A", "env[1]": "B"},
			expected: map[string]any{
				"env": []any{"A", "B"},
			},
		},
		{
			name: "list of maps",
			input: map[string]string{
				"ports[0].name": "http",
				"ports[0].port": "80",
				"ports[1].name": "admin",
			},
			expected: map[string]any{
				"ports": []any{
					map[string]any{"name": "http", "port": "80"},
					map[string]any{"name": "admin"},
				},
			},
		},
		{
			name: "nested lists",
			input: map[string]string{
				"matrix[0][0]": "a",
				"matrix[0][1]": "b",
				"matrix[1][0]": "c",
			},
			expected: map[string]any{
				"matrix": []any{
					[]any{"a", "b"},
					[]any{"c"},
				},
			},
		},
		{
			name:  "leaf conflict list wins",
			input: map[string]string{"env": "leaf", "env[0]": "A"},
			expected: map[string]any{
				"env": []any{"A"},
			},
		},
		{
			name:  "sparse list pads nil",
			input: map[string]string{"env[1]": "B"},
			expected: map[string]any{
				"env": []any{nil, "B"},
			},
		},
		{
			name:  "mixed with normal maps",
			input: map[string]string{"image.tag": "v1", "env[0]": "A"},
			expected: map[string]any{
				"image": map[string]any{"tag": "v1"},
				"env":   []any{"A"},
			},
		},
		{
			name:  "invalid array syntax stays literal",
			input: map[string]string{"foo[bar]": "x", "foo[0]x": "y"},
			expected: map[string]any{
				"foo[bar]": "x",
				"foo[0]x":  "y",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := UnflattenYAML(tt.input, false)
			if !reflect.DeepEqual(result, tt.expected) {
				t.Errorf("UnflattenYAML() = %#v, want %#v", result, tt.expected)
			}
		})
	}
}

func TestUnflattenYAMLArraysDeterministic(t *testing.T) {
	input := map[string]string{
		"ports[0].name": "http",
		"ports[0].port": "80",
		"ports[1].name": "admin",
		"matrix[0][0]":  "a",
		"matrix[0][1]":  "b",
		"matrix[1][0]":  "c",
		"env":           "leaf",
		"env[0]":        "A",
		"env[1]":        "B",
	}
	expected := map[string]any{
		"ports": []any{
			map[string]any{"name": "http", "port": "80"},
			map[string]any{"name": "admin"},
		},
		"matrix": []any{
			[]any{"a", "b"},
			[]any{"c"},
		},
		"env": []any{"A", "B"},
	}

	for i := range 1000 {
		result := UnflattenYAML(input, false)
		if !reflect.DeepEqual(result, expected) {
			t.Fatalf("non-deterministic result on iteration %d: got %#v", i, result)
		}
	}
}

func saveYAMLTestData() map[string]any {
	return map[string]any{
		"password": "secret",
		"replicas": "3",
	}
}

func tempFiles(t *testing.T, dir string) []string {
	t.Helper()

	matches, err := filepath.Glob(filepath.Join(dir, ".argo-values-*"))
	if err != nil {
		t.Fatalf("failed to glob temp files: %v", err)
	}
	return matches
}

func TestSaveYAMLCreatesFileWith0600(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "values.yaml")

	data := saveYAMLTestData()
	if err := SaveYAML(data, path); err != nil {
		t.Fatalf("SaveYAML returned error: %v", err)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("failed to stat %s: %v", path, err)
	}
	if got := info.Mode().Perm(); got != 0600 {
		t.Errorf("file mode = %o, want %o", got, 0600)
	}

	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed to read %s: %v", path, err)
	}
	want := "password: secret\nreplicas: \"3\"\n"
	if string(content) != want {
		t.Errorf("file content = %q, want %q", string(content), want)
	}
}

func TestSaveYAMLTightensExistingPermissions(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "values.yaml")
	if err := os.WriteFile(path, []byte("old: value\n"), 0644); err != nil {
		t.Fatalf("failed to create existing file: %v", err)
	}
	if err := os.Chmod(path, 0644); err != nil {
		t.Fatalf("failed to chmod existing file: %v", err)
	}

	if err := SaveYAML(saveYAMLTestData(), path); err != nil {
		t.Fatalf("SaveYAML returned error: %v", err)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("failed to stat %s: %v", path, err)
	}
	if got := info.Mode().Perm(); got != 0600 {
		t.Errorf("file mode = %o, want %o", got, 0600)
	}
}

func TestSaveYAMLRefusesSymlink(t *testing.T) {
	dir := t.TempDir()
	outside := t.TempDir()
	target := filepath.Join(outside, "target.yaml")
	const original = "keep: me\n"
	if err := os.WriteFile(target, []byte(original), 0600); err != nil {
		t.Fatalf("failed to create target: %v", err)
	}

	link := filepath.Join(dir, "values.yaml")
	if err := os.Symlink(target, link); err != nil {
		t.Fatalf("failed to create symlink: %v", err)
	}

	if err := SaveYAML(saveYAMLTestData(), link); err == nil {
		t.Fatal("SaveYAML returned nil error for symlink, want error")
	}

	content, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("failed to read target: %v", err)
	}
	if string(content) != original {
		t.Errorf("target content = %q, want unchanged %q", string(content), original)
	}

	info, err := os.Lstat(link)
	if err != nil {
		t.Fatalf("failed to lstat symlink: %v", err)
	}
	if info.Mode()&os.ModeSymlink == 0 {
		t.Errorf("link is no longer a symlink, mode = %v", info.Mode())
	}

	if leftovers := tempFiles(t, dir); len(leftovers) != 0 {
		t.Errorf("temp files left behind after error: %v", leftovers)
	}
}

func TestSaveYAMLRefusesDirectory(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "values.yaml")
	if err := os.Mkdir(target, 0755); err != nil {
		t.Fatalf("failed to create directory: %v", err)
	}

	if err := SaveYAML(saveYAMLTestData(), target); err == nil {
		t.Fatal("SaveYAML returned nil error for directory, want error")
	}

	if leftovers := tempFiles(t, dir); len(leftovers) != 0 {
		t.Errorf("temp files left behind after error: %v", leftovers)
	}
}

func TestSaveYAMLLeavesNoTempFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "values.yaml")

	if err := SaveYAML(saveYAMLTestData(), path); err != nil {
		t.Fatalf("SaveYAML returned error: %v", err)
	}

	if leftovers := tempFiles(t, dir); len(leftovers) != 0 {
		t.Errorf("temp files left behind after write: %v", leftovers)
	}
}

// Helper function to compare maps recursively
func compareMaps(t *testing.T, result, expected map[string]any) {
	if len(result) != len(expected) {
		t.Errorf("MergeYAML() length = %d, want %d", len(result), len(expected))
		return
	}

	for k, expectedVal := range expected {
		if resultVal, exists := result[k]; !exists {
			t.Errorf("MergeYAML() missing key %s", k)
		} else {
			switch ev := expectedVal.(type) {
			case map[string]any:
				if rv, ok := resultVal.(map[string]any); ok {
					compareMaps(t, rv, ev)
				} else {
					t.Errorf("MergeYAML()[%s] type mismatch: expected map, got %T", k, resultVal)
				}
			default:
				if resultVal != expectedVal {
					t.Errorf("MergeYAML()[%s] = %v, want %v", k, resultVal, expectedVal)
				}
			}
		}
	}
}
