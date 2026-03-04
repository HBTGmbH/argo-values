package helm

import (
	"os"
	"path/filepath"
	"testing"
)

func TestReadHelmIgnore(t *testing.T) {
	tests := []struct {
		name        string
		setupFunc   func(dir string) error
		expectError bool
		expectedLen int
	}{
		{
			name: "non-existent .helmignore",
			setupFunc: func(dir string) error {
				return nil // Don't create any file
			},
			expectError: false,
			expectedLen: 0,
		},
		{
			name: "empty .helmignore",
			setupFunc: func(dir string) error {
				return os.WriteFile(filepath.Join(dir, ".helmignore"), []byte(""), 0644)
			},
			expectError: false,
			expectedLen: 0,
		},
		{
			name: ".helmignore with patterns",
			setupFunc: func(dir string) error {
				content := "# comment\ntemp/\n*.log\n\nconfig.yaml\n"
				return os.WriteFile(filepath.Join(dir, ".helmignore"), []byte(content), 0644)
			},
			expectError: false,
			expectedLen: 3, // temp/, *.log, config.yaml (comment and empty lines ignored)
		},
		{
			name: ".helmignore with only comments",
			setupFunc: func(dir string) error {
				content := "# comment 1\n# comment 2\n"
				return os.WriteFile(filepath.Join(dir, ".helmignore"), []byte(content), 0644)
			},
			expectError: false,
			expectedLen: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tempDir := t.TempDir()
			if err := tt.setupFunc(tempDir); err != nil {
				t.Fatalf("setup failed: %v", err)
			}

			result, err := ReadHelmIgnore(tempDir)
			if (err != nil) != tt.expectError {
				t.Errorf("ReadHelmIgnore() error = %v, expectError %v", err, tt.expectError)
				return
			}

			if result == nil && tt.expectedLen != 0 {
				t.Errorf("ReadHelmIgnore() = nil, want non-nil with length %d", tt.expectedLen)
				return
			}

			if result != nil && len(*result) != tt.expectedLen {
				t.Errorf("ReadHelmIgnore() length = %d, want %d", len(*result), tt.expectedLen)
			}
		})
	}
}

func TestShouldIgnore(t *testing.T) {
	tests := []struct {
		name     string
		patterns IgnoreFile
		filePath string
		baseDir  string
		expected bool
	}{
		{
			name:     "empty patterns",
			patterns: IgnoreFile{},
			filePath: "some/file.txt",
			baseDir:  "some",
			expected: false,
		},
		{
			name:     "exact match",
			patterns: IgnoreFile{"config.yaml"},
			filePath: "config.yaml",
			baseDir:  ".",
			expected: true,
		},
		{
			name:     "directory match",
			patterns: IgnoreFile{"temp/"},
			filePath: "temp/file.txt",
			baseDir:  ".",
			expected: true,
		},
		{
			name:     "directory no match",
			patterns: IgnoreFile{"temp/"},
			filePath: "other/file.txt",
			baseDir:  ".",
			expected: false,
		},
		{
			name:     "wildcard match",
			patterns: IgnoreFile{"*.log"},
			filePath: "app.log",
			baseDir:  ".",
			expected: true,
		},
		{
			name:     "wildcard no match",
			patterns: IgnoreFile{"*.log"},
			filePath: "app.txt",
			baseDir:  ".",
			expected: false,
		},
		{
			name:     "subdirectory file",
			patterns: IgnoreFile{"*.log"},
			filePath: "logs/app.log",
			baseDir:  ".",
			expected: true,
		},
		{
			name:     "multiple patterns - should match",
			patterns: IgnoreFile{"temp/", "*.log", "config.yaml"},
			filePath: "temp/cache",
			baseDir:  ".",
			expected: true,
		},
		{
			name:     "multiple patterns - should not match",
			patterns: IgnoreFile{"temp/", "*.log", "config.yaml"},
			filePath: "src/main.go",
			baseDir:  ".",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.patterns.ShouldIgnore(tt.filePath, tt.baseDir)
			if result != tt.expected {
				t.Errorf("ShouldIgnore() = %v, want %v for path %s with patterns %v", result, tt.expected, tt.filePath, tt.patterns)
			}
		})
	}
}

func TestMatchesPattern(t *testing.T) {
	tests := []struct {
		name     string
		path     string
		pattern  string
		expected bool
	}{
		{
			name:     "exact match",
			path:     "config.yaml",
			pattern:  "config.yaml",
			expected: true,
		},
		{
			name:     "no match",
			path:     "config.yaml",
			pattern:  "values.yaml",
			expected: false,
		},
		{
			name:     "directory match",
			path:     "temp/cache/file",
			pattern:  "temp/",
			expected: true,
		},
		{
			name:     "directory no match",
			path:     "other/file",
			pattern:  "temp/",
			expected: false,
		},
		{
			name:     "wildcard match",
			path:     "application.log",
			pattern:  "*.log",
			expected: true,
		},
		{
			name:     "wildcard no match",
			path:     "application.txt",
			pattern:  "*.log",
			expected: false,
		},
		{
			name:     "wildcard in middle",
			path:     "app-debug.log",
			pattern:  "*-debug.log",
			expected: true,
		},
		{
			name:     "wildcard multiple",
			path:     "app-debug-2023.log",
			pattern:  "*-debug-*.log",
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := matchesPattern(tt.path, tt.pattern)
			if result != tt.expected {
				t.Errorf("matchesPattern() = %v, want %v for path %s pattern %s", result, tt.expected, tt.path, tt.pattern)
			}
		})
	}
}
