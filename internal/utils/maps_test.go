package utils

import (
	"testing"
)

func TestGetStringFromMap(t *testing.T) {
	tests := []struct {
		name     string
		input    map[string]any
		key      string
		expected string
	}{
		{
			name:     "existing string key",
			input:    map[string]any{"key": "value"},
			key:      "key",
			expected: "value",
		},
		{
			name:     "non-existing key",
			input:    map[string]any{"other": "value"},
			key:      "key",
			expected: "",
		},
		{
			name:     "non-string value",
			input:    map[string]any{"key": 123},
			key:      "key",
			expected: "",
		},
		{
			name:     "empty map",
			input:    map[string]any{},
			key:      "key",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := GetStringFromMap(tt.input, tt.key)
			if result != tt.expected {
				t.Errorf("GetStringFromMap() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestGetArrayFromMap(t *testing.T) {
	tests := []struct {
		name     string
		input    map[string]any
		key      string
		expected []string
	}{
		{
			name:     "existing string array",
			input:    map[string]any{"key": []any{"a", "b", "c"}},
			key:      "key",
			expected: []string{"a", "b", "c"},
		},
		{
			name:     "non-existing key",
			input:    map[string]any{"other": []any{"a", "b"}},
			key:      "key",
			expected: []string{},
		},
		{
			name:     "non-array value",
			input:    map[string]any{"key": "not an array"},
			key:      "key",
			expected: []string{},
		},
		{
			name:     "array with non-string element",
			input:    map[string]any{"key": []any{"a", 123, "c"}},
			key:      "key",
			expected: []string{},
		},
		{
			name:     "empty array",
			input:    map[string]any{"key": []any{}},
			key:      "key",
			expected: []string{},
		},
		{
			name:     "empty map",
			input:    map[string]any{},
			key:      "key",
			expected: []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := GetArrayFromMap(tt.input, tt.key)
			if len(result) != len(tt.expected) {
				t.Errorf("GetArrayFromMap() length = %d, want %d", len(result), len(tt.expected))
				return
			}
			for i := range result {
				if result[i] != tt.expected[i] {
					t.Errorf("GetArrayFromMap()[%d] = %v, want %v", i, result[i], tt.expected[i])
				}
			}
		})
	}
}
