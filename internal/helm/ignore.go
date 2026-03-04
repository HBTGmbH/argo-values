package helm

import (
	"bufio"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

type IgnoreFile []string

// ReadHelmIgnore reads .helmignore file and returns patterns to ignore
func ReadHelmIgnore(directory string) (*IgnoreFile, error) {
	helmIgnorePath := filepath.Join(directory, ".helmignore")

	// Check if .helmignore exists
	if _, err := os.Stat(helmIgnorePath); os.IsNotExist(err) {
		return nil, nil // No .helmignore file
	}

	file, err := os.Open(helmIgnorePath)
	if err != nil {
		return nil, err
	}
	defer func(file *os.File) {
		_ = file.Close()
	}(file)

	var patterns IgnoreFile
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		// Skip empty lines and comments
		if line != "" && !strings.HasPrefix(line, "#") {
			patterns = append(patterns, line)
		}
	}

	return &patterns, scanner.Err()
}

// ShouldIgnore checks if a file path should be ignored based on .helmignore patterns
func (i *IgnoreFile) ShouldIgnore(filePath, baseDirectory string) bool {
	if len(*i) == 0 {
		return false
	}

	// Get the relative path from the base directory
	relPath, err := filepath.Rel(baseDirectory, filePath)
	if err != nil {
		return false
	}

	// Convert to forward slashes for pattern matching
	relPath = filepath.ToSlash(relPath)

	// Check against each pattern
	for _, pattern := range *i {
		if matchesPattern(relPath, pattern) {
			return true
		}
	}

	return false
}

// matchesPattern checks if a path matches a .helmignore pattern
func matchesPattern(path, pattern string) bool {
	// Simple exact match
	if path == pattern {
		return true
	}

	// Directory match (the pattern ends with /)
	if strings.HasSuffix(pattern, "/") {
		if strings.HasPrefix(path, pattern) {
			return true
		}
	}

	// Wildcard match (*)
	if strings.Contains(pattern, "*") {
		regexPattern := regexp.QuoteMeta(pattern)
		regexPattern = strings.ReplaceAll(regexPattern, "\\*", ".*")
		matched, _ := regexp.MatchString("^"+regexPattern+"$", path)
		return matched
	}

	return false
}
