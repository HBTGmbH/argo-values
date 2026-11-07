package helm

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
)

func TestReadHelmIgnore(t *testing.T) {
	tests := []struct {
		name        string
		content     *string // nil means no .helmignore file
		expectError bool
		expectedLen int
	}{
		{
			name:        "non-existent .helmignore",
			content:     nil,
			expectError: false,
			expectedLen: 1, // only the default pattern
		},
		{
			name:        "empty .helmignore",
			content:     new(""),
			expectError: false,
			expectedLen: 1,
		},
		{
			name:        ".helmignore with patterns",
			content:     new("# comment\ntemp/\n*.log\n\nconfig.yaml\n"),
			expectError: false,
			expectedLen: 4, // temp/, *.log, config.yaml + default
		},
		{
			name:        ".helmignore with only comments",
			content:     new("# comment 1\n# comment 2\n"),
			expectError: false,
			expectedLen: 1,
		},
		{
			name:        "UTF-8 BOM is stripped from the first line",
			content:     new("\xEF\xBB\xBFtemp/\n*.log\n"),
			expectError: false,
			expectedLen: 3,
		},
		{
			name:        "double-star is rejected",
			content:     new("**/*.txt\n"),
			expectError: true,
		},
		{
			name:        "invalid bracket pattern is rejected",
			content:     new("[\n"),
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tempDir := t.TempDir()
			if tt.content != nil {
				if err := os.WriteFile(filepath.Join(tempDir, ".helmignore"), []byte(*tt.content), 0644); err != nil {
					t.Fatalf("setup failed: %v", err)
				}
			}

			result, err := ReadHelmIgnore(tempDir)
			if (err != nil) != tt.expectError {
				t.Errorf("ReadHelmIgnore() error = %v, expectError %v", err, tt.expectError)
				return
			}

			if tt.expectError {
				if result != nil {
					t.Errorf("ReadHelmIgnore() = %v, want nil on error", result)
				}
				return
			}

			if result == nil {
				t.Fatal("ReadHelmIgnore() = nil, want non-nil")
			}
			if len(*result) != tt.expectedLen {
				t.Errorf("ReadHelmIgnore() length = %d, want %d (%v)", len(*result), tt.expectedLen, *result)
			}
		})
	}
}

// strPtr returns a pointer to the given string, used to distinguish "no file"
// from "empty file" in the test table.
//
//go:fix inline
func strPtr(s string) *string {
	return new(s)
}

func TestReadHelmIgnoreStripsBOM(t *testing.T) {
	tempDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(tempDir, ".helmignore"), []byte("\xEF\xBB\xBFtemp/\n"), 0644); err != nil {
		t.Fatalf("setup failed: %v", err)
	}

	result, err := ReadHelmIgnore(tempDir)
	if err != nil {
		t.Fatalf("ReadHelmIgnore() error = %v", err)
	}
	if len(*result) == 0 || (*result)[0] != "temp/" {
		t.Errorf("first pattern = %q, want %q", (*result)[0], "temp/")
	}
}

func TestReadHelmIgnoreAddsDefaultPattern(t *testing.T) {
	tempDir := t.TempDir()

	result, err := ReadHelmIgnore(tempDir)
	if err != nil {
		t.Fatalf("ReadHelmIgnore() error = %v", err)
	}

	found := slices.Contains(*result, "templates/.?*")
	if !found {
		t.Errorf("ReadHelmIgnore() = %v, want default pattern %q", *result, "templates/.?*")
	}

	if !result.ShouldIgnore(filepath.Join(tempDir, "templates", ".gitkeep"), tempDir, false) {
		t.Error("templates/.gitkeep should be ignored by the default pattern")
	}
	if result.ShouldIgnore(filepath.Join(tempDir, "templates", "gitkeep"), tempDir, false) {
		t.Error("templates/gitkeep should not be ignored by the default pattern")
	}
}

func TestShouldIgnore(t *testing.T) {
	tests := []struct {
		name     string
		patterns IgnoreFile
		filePath string
		baseDir  string
		isDir    bool
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
			name:     "exact match on basename",
			patterns: IgnoreFile{"config.yaml"},
			filePath: "config.yaml",
			baseDir:  ".",
			expected: true,
		},
		{
			name:     "exact match on basename in subdirectory",
			patterns: IgnoreFile{"foo.txt"},
			filePath: "sub/foo.txt",
			baseDir:  ".",
			expected: true,
		},
		{
			name:     "path pattern matches only direct child",
			patterns: IgnoreFile{"templates/*.yaml"},
			filePath: "templates/a.yaml",
			baseDir:  ".",
			expected: true,
		},
		{
			name:     "path pattern does not cross directory boundaries",
			patterns: IgnoreFile{"templates/*.yaml"},
			filePath: "templates/sub/b.yaml",
			baseDir:  ".",
			expected: false,
		},
		{
			name:     "basename wildcard matches nested file",
			patterns: IgnoreFile{"*.log"},
			filePath: "logs/app.log",
			baseDir:  ".",
			expected: true,
		},
		{
			name:     "basename wildcard no match",
			patterns: IgnoreFile{"*.log"},
			filePath: "app.txt",
			baseDir:  ".",
			expected: false,
		},
		{
			name:     "directory pattern matches nested directory",
			patterns: IgnoreFile{"foo/"},
			filePath: "sub/foo",
			baseDir:  ".",
			isDir:    true,
			expected: true,
		},
		{
			name:     "directory pattern does not match file",
			patterns: IgnoreFile{"foo/"},
			filePath: "sub/foo",
			baseDir:  ".",
			isDir:    false,
			expected: false,
		},
		{
			name:     "root-anchored pattern matches root file",
			patterns: IgnoreFile{"/root.txt"},
			filePath: "root.txt",
			baseDir:  ".",
			expected: true,
		},
		{
			name:     "root-anchored pattern does not match nested file",
			patterns: IgnoreFile{"/root.txt"},
			filePath: "sub/root.txt",
			baseDir:  ".",
			expected: false,
		},
		{
			name:     "question mark wildcard",
			patterns: IgnoreFile{"*.t?t"},
			filePath: "a.txt",
			baseDir:  ".",
			expected: true,
		},
		{
			name:     "character class wildcard",
			patterns: IgnoreFile{"file[0-9].txt"},
			filePath: "file1.txt",
			baseDir:  ".",
			expected: true,
		},
		{
			name:     "multiple patterns - should match",
			patterns: IgnoreFile{"temp/", "*.log", "config.yaml"},
			filePath: "temp",
			baseDir:  ".",
			isDir:    true,
			expected: true,
		},
		{
			name:     "multiple patterns - should not match",
			patterns: IgnoreFile{"temp/", "*.log", "config.yaml"},
			filePath: "src/main.go",
			baseDir:  ".",
			expected: false,
		},
		{
			name:     "base directory is never ignored",
			patterns: IgnoreFile{"*"},
			filePath: ".",
			baseDir:  ".",
			isDir:    true,
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.patterns.ShouldIgnore(tt.filePath, tt.baseDir, tt.isDir)
			if result != tt.expected {
				t.Errorf("ShouldIgnore() = %v, want %v for path %s (isDir=%v) with patterns %v", result, tt.expected, tt.filePath, tt.isDir, tt.patterns)
			}
		})
	}
}

func TestShouldIgnoreRelativeToBaseDirectory(t *testing.T) {
	patterns := IgnoreFile{"templates/*.yaml"}
	baseDir := filepath.Join("some", "chart")

	if !patterns.ShouldIgnore(filepath.Join(baseDir, "templates", "a.yaml"), baseDir, false) {
		t.Error("ShouldIgnore() = false, want true for direct child relative to base directory")
	}
	if patterns.ShouldIgnore(filepath.Join(baseDir, "templates", "sub", "b.yaml"), baseDir, false) {
		t.Error("ShouldIgnore() = true, want false for nested child")
	}
}

func TestReadHelmIgnoreMissingFileReturnsNonNil(t *testing.T) {
	tempDir := t.TempDir()

	result, err := ReadHelmIgnore(tempDir)
	if err != nil {
		t.Fatalf("ReadHelmIgnore() error = %v, want nil", err)
	}
	if result == nil {
		t.Fatal("ReadHelmIgnore() = nil, want non-nil IgnoreFile with default pattern")
	}
	if len(*result) != 1 {
		t.Errorf("ReadHelmIgnore() length = %d, want 1", len(*result))
	}
}

func TestShouldIgnoreNilReceiver(t *testing.T) {
	var i *IgnoreFile

	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("ShouldIgnore() panicked on nil receiver: %v", r)
		}
	}()

	if i.ShouldIgnore("some/file.txt", ".", false) {
		t.Error("ShouldIgnore() on nil receiver = true, want false")
	}
}

func TestShouldIgnoreEmptyIgnoreFile(t *testing.T) {
	i := IgnoreFile{}

	if i.ShouldIgnore("some/file.txt", ".", false) {
		t.Error("ShouldIgnore() on empty IgnoreFile = true, want false")
	}
}

func TestCompilePattern(t *testing.T) {
	tests := []struct {
		name           string
		pattern        string
		path           string
		expectedMatch  bool
		expectedNegate bool
		expectedDir    bool
		expectedNil    bool
		expectError    bool
	}{
		{
			name:          "exact basename",
			pattern:       "config.yaml",
			path:          "config.yaml",
			expectedMatch: true,
		},
		{
			name:          "basename matches nested path",
			pattern:       "config.yaml",
			path:          "sub/config.yaml",
			expectedMatch: true,
		},
		{
			name:          "path pattern direct child",
			pattern:       "templates/*.yaml",
			path:          "templates/a.yaml",
			expectedMatch: true,
		},
		{
			name:          "path pattern nested child",
			pattern:       "templates/*.yaml",
			path:          "templates/sub/b.yaml",
			expectedMatch: false,
		},
		{
			name:          "directory marker",
			pattern:       "foo/",
			path:          "sub/foo",
			expectedMatch: true,
			expectedDir:   true,
		},
		{
			name:           "negation",
			pattern:        "!keep.txt",
			path:           "keep.txt",
			expectedMatch:  true,
			expectedNegate: true,
		},
		{
			name:          "root anchored matches root",
			pattern:       "/root.txt",
			path:          "root.txt",
			expectedMatch: true,
		},
		{
			name:          "root anchored rejects nested",
			pattern:       "/root.txt",
			path:          "sub/root.txt",
			expectedMatch: false,
		},
		{
			name:          "question mark wildcard",
			pattern:       "*.t?t",
			path:          "a.txt",
			expectedMatch: true,
		},
		{
			name:          "character class",
			pattern:       "file[0-9].txt",
			path:          "file1.txt",
			expectedMatch: true,
		},
		{
			name:        "comment yields no match function",
			pattern:     "# comment",
			expectedNil: true,
		},
		{
			name:        "empty pattern yields no match function",
			pattern:     "",
			expectedNil: true,
		},
		{
			name:        "double-star is rejected",
			pattern:     "**/*.txt",
			expectError: true,
		},
		{
			name:        "invalid bracket is rejected",
			pattern:     "[",
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			match, negate, mustDir, err := compilePattern(tt.pattern)
			if (err != nil) != tt.expectError {
				t.Fatalf("compilePattern() error = %v, expectError %v", err, tt.expectError)
			}
			if tt.expectError {
				return
			}
			if tt.expectedNil {
				if match != nil {
					t.Fatalf("compilePattern() match = non-nil, want nil")
				}
				return
			}
			if match == nil {
				t.Fatal("compilePattern() match = nil, want non-nil")
			}
			if negate != tt.expectedNegate {
				t.Errorf("compilePattern() negate = %v, want %v", negate, tt.expectedNegate)
			}
			if mustDir != tt.expectedDir {
				t.Errorf("compilePattern() mustDir = %v, want %v", mustDir, tt.expectedDir)
			}
			if got := match(tt.path); got != tt.expectedMatch {
				t.Errorf("match(%q) = %v, want %v", tt.path, got, tt.expectedMatch)
			}
		})
	}
}
