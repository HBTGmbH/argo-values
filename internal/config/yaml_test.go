package config

import (
	"testing"
)

func TestParseYAML(t *testing.T) {
	// Simple happy path test for ParseYAML
	result := ParseYAML("test", "key: value")

	if len(result) != 1 {
		t.Fatalf("Expected 1 key, got %d", len(result))
	}

	if val, exists := result["key"]; !exists {
		t.Error("Expected key 'key' not found")
	} else if val != "value" {
		t.Errorf("Expected value 'value', got %v", val)
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
		input    map[string]interface{}
		expected map[string]string
	}{
		{
			name:     "flat structure",
			input:    map[string]interface{}{"key1": "value1", "key2": "value2"},
			expected: map[string]string{"KEY1": "value1", "KEY2": "value2"},
		},
		{
			name:     "nested structure",
			input:    map[string]interface{}{"outer": map[string]interface{}{"inner": "value"}},
			expected: map[string]string{"OUTER_INNER": "value"},
		},
		{
			name:     "deeply nested",
			input:    map[string]interface{}{"a": map[string]interface{}{"b": map[string]interface{}{"c": "deep"}}},
			expected: map[string]string{"A_B_C": "deep"},
		},
		{
			name:     "mixed types",
			input:    map[string]interface{}{"string": "text", "number": 42, "bool": true},
			expected: map[string]string{"STRING": "text", "NUMBER": "42", "BOOL": "true"},
		},
		{
			name:     "empty map",
			input:    map[string]interface{}{},
			expected: map[string]string{},
		},
		{
			name: "complex nested",
			input: map[string]interface{}{
				"database": map[string]interface{}{
					"host": "localhost",
					"port": 5432,
					"credentials": map[string]interface{}{
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
		a        map[string]interface{}
		b        map[string]interface{}
		expected map[string]interface{}
	}{
		{
			name:     "simple merge",
			a:        map[string]interface{}{"key1": "value1"},
			b:        map[string]interface{}{"key2": "value2"},
			expected: map[string]interface{}{"key1": "value1", "key2": "value2"},
		},
		{
			name:     "overwrite value",
			a:        map[string]interface{}{"key": "old"},
			b:        map[string]interface{}{"key": "new"},
			expected: map[string]interface{}{"key": "new"},
		},
		{
			name:     "merge nested maps",
			a:        map[string]interface{}{"outer": map[string]interface{}{"inner1": "value1"}},
			b:        map[string]interface{}{"outer": map[string]interface{}{"inner2": "value2"}},
			expected: map[string]interface{}{"outer": map[string]interface{}{"inner1": "value1", "inner2": "value2"}},
		},
		{
			name:     "deep merge",
			a:        map[string]interface{}{"a": map[string]interface{}{"b": map[string]interface{}{"c": "old"}}},
			b:        map[string]interface{}{"a": map[string]interface{}{"b": map[string]interface{}{"d": "new"}}},
			expected: map[string]interface{}{"a": map[string]interface{}{"b": map[string]interface{}{"c": "old", "d": "new"}}},
		},
		{
			name:     "empty maps",
			a:        map[string]interface{}{},
			b:        map[string]interface{}{"key": "value"},
			expected: map[string]interface{}{"key": "value"},
		},
		{
			name:     "overwrite nested with simple",
			a:        map[string]interface{}{"key": map[string]interface{}{"nested": "value"}},
			b:        map[string]interface{}{"key": "simple"},
			expected: map[string]interface{}{"key": "simple"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := MergeYAML(tt.a, tt.b)
			compareMaps(t, result, tt.expected)
		})
	}
}

func TestUnflattenSanitizedYAML(t *testing.T) {
	tests := []struct {
		name     string
		input    map[string]string
		expected map[string]interface{}
	}{
		{
			name:     "single flat key",
			input:    map[string]string{"REPLICAS": "3"},
			expected: map[string]interface{}{"replicas": "3"},
		},
		{
			name:     "nested key",
			input:    map[string]string{"RESOURCES_REQUESTS_CPU": "123m"},
			expected: map[string]interface{}{"resources": map[string]interface{}{"requests": map[string]interface{}{"cpu": "123m"}}},
		},
		{
			name: "multiple nested keys",
			input: map[string]string{
				"RESOURCES_REQUESTS_CPU":    "123m",
				"RESOURCES_REQUESTS_MEMORY": "256Mi",
			},
			expected: map[string]interface{}{
				"resources": map[string]interface{}{
					"requests": map[string]interface{}{
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
			expected: map[string]interface{}{
				"replicas": "3",
				"resources": map[string]interface{}{
					"requests": map[string]interface{}{
						"cpu":    "123m",
						"memory": "256Mi",
					},
				},
			},
		},
		{
			name:     "empty map",
			input:    map[string]string{},
			expected: map[string]interface{}{},
		},
		{
			name: "complex nested",
			input: map[string]string{
				"DATABASE_HOST":                 "localhost",
				"DATABASE_PORT":                 "5432",
				"DATABASE_CREDENTIALS_USERNAME": "admin",
				"DATABASE_CREDENTIALS_PASSWORD": "secret",
			},
			expected: map[string]interface{}{
				"database": map[string]interface{}{
					"host": "localhost",
					"port": "5432",
					"credentials": map[string]interface{}{
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
			expected: map[string]interface{}{
				"a": map[string]interface{}{
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
		expected map[string]interface{}
	}{
		{
			name:     "single flat key",
			input:    map[string]string{"replicas": "3"},
			expected: map[string]interface{}{"replicas": "3"},
		},
		{
			name:     "nested key",
			input:    map[string]string{"resources.requests.cpu": "123m"},
			expected: map[string]interface{}{"resources": map[string]interface{}{"requests": map[string]interface{}{"cpu": "123m"}}},
		},
		{
			name: "multiple nested keys",
			input: map[string]string{
				"resources.requests.cpu":    "123m",
				"resources.requests.memory": "256Mi",
			},
			expected: map[string]interface{}{
				"resources": map[string]interface{}{
					"requests": map[string]interface{}{
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
			expected: map[string]interface{}{
				"replicas": "3",
				"resources": map[string]interface{}{
					"requests": map[string]interface{}{
						"cpu":    "123m",
						"memory": "256Mi",
					},
				},
			},
		},
		{
			name:     "empty map",
			input:    map[string]string{},
			expected: map[string]interface{}{},
		},
		{
			name: "complex nested",
			input: map[string]string{
				"database.host":                 "localhost",
				"database.port":                 "5432",
				"database.credentials.username": "admin",
				"database.credentials.password": "secret",
			},
			expected: map[string]interface{}{
				"database": map[string]interface{}{
					"host": "localhost",
					"port": "5432",
					"credentials": map[string]interface{}{
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
			expected: map[string]interface{}{
				"a": map[string]interface{}{
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
	input := map[string]interface{}{
		"resources": map[string]interface{}{
			"requests": map[string]interface{}{
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

// Helper function to compare maps recursively
func compareMaps(t *testing.T, result, expected map[string]interface{}) {
	if len(result) != len(expected) {
		t.Errorf("MergeYAML() length = %d, want %d", len(result), len(expected))
		return
	}

	for k, expectedVal := range expected {
		if resultVal, exists := result[k]; !exists {
			t.Errorf("MergeYAML() missing key %s", k)
		} else {
			switch ev := expectedVal.(type) {
			case map[string]interface{}:
				if rv, ok := resultVal.(map[string]interface{}); ok {
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
