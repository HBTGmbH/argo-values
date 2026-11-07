package utils

import (
	"testing"
)

func TestTrimmed(t *testing.T) {
	tests := []struct {
		name     string
		input    []string
		expected []string
	}{
		{
			name:     "trims surrounding whitespace",
			input:    []string{" a ", "\tb", "c\n"},
			expected: []string{"a", "b", "c"},
		},
		{
			name:     "drops empty and whitespace-only entries",
			input:    []string{"a", "", " ", "\t", "b"},
			expected: []string{"a", "b"},
		},
		{
			name:     "empty slice returns empty non-nil slice",
			input:    []string{},
			expected: []string{},
		},
		{
			name:     "all empty returns empty non-nil slice",
			input:    []string{"", " "},
			expected: []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := Trimmed(tt.input)
			if result == nil {
				t.Fatal("Trimmed() returned nil, want non-nil slice")
			}
			if len(result) != len(tt.expected) {
				t.Fatalf("Trimmed() length = %d, want %d", len(result), len(tt.expected))
			}
			for i := range result {
				if result[i] != tt.expected[i] {
					t.Errorf("Trimmed()[%d] = %q, want %q", i, result[i], tt.expected[i])
				}
			}
		})
	}
}
