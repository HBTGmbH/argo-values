package helm

import (
	"bufio"
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
)

// IgnoreFile holds the raw patterns read from a .helmignore file plus the
// default Helm pattern. Patterns are compiled on demand by ShouldIgnore.
type IgnoreFile []string

// defaultIgnorePattern is appended by Helm's loader via AddDefaults(). It
// ignores dotfiles directly inside templates/ so they cannot be injected into
// the rendered chart.
const defaultIgnorePattern = "templates/.?*"

var utf8BOM = []byte{0xEF, 0xBB, 0xBF}

// ReadHelmIgnore reads the .helmignore file in directory and returns its
// patterns. If the file does not exist, only the default Helm pattern is
// returned. Invalid patterns (e.g. "**" or "[") yield a nil *IgnoreFile and an
// error. Other errors (stat/open/scan) are returned alongside a nil
// *IgnoreFile as well.
func ReadHelmIgnore(directory string) (*IgnoreFile, error) {
	helmIgnorePath := filepath.Join(directory, ".helmignore")

	patterns := IgnoreFile{}

	info, err := os.Stat(helmIgnorePath)
	switch {
	case err == nil && !info.IsDir():
		file, openErr := os.Open(helmIgnorePath)
		if openErr != nil {
			return nil, openErr
		}
		defer func(file *os.File) {
			_ = file.Close()
		}(file)

		scanner := bufio.NewScanner(file)
		firstLine := true
		for scanner.Scan() {
			lineBytes := scanner.Bytes()
			if firstLine {
				// Strip a UTF-8 BOM, which may only appear at the very
				// beginning of the file.
				lineBytes = bytes.TrimPrefix(lineBytes, utf8BOM)
				firstLine = false
			}

			line := strings.TrimSpace(string(lineBytes))
			// Skip empty lines and comments.
			if line == "" || strings.HasPrefix(line, "#") {
				continue
			}
			patterns = append(patterns, line)
		}
		if scanErr := scanner.Err(); scanErr != nil {
			return nil, scanErr
		}
	case err != nil && !os.IsNotExist(err):
		return nil, err
	}

	// Validate every pattern exactly like Helm does while parsing a rules file.
	for _, pattern := range patterns {
		if _, _, _, compileErr := compilePattern(pattern); compileErr != nil {
			return nil, compileErr
		}
	}

	// Helm's loader always adds its defaults after reading the file.
	patterns = append(patterns, defaultIgnorePattern)

	return &patterns, nil
}

// ShouldIgnore checks if a file or directory should be ignored based on the
// .helmignore patterns. It is nil-safe: a nil receiver or an empty pattern set
// yields false. The evaluation order and negation semantics mirror Helm.
func (i *IgnoreFile) ShouldIgnore(filePath, baseDirectory string, isDir bool) bool {
	if i == nil || len(*i) == 0 {
		return false
	}

	// Get the relative path from the base directory.
	relPath, err := filepath.Rel(baseDirectory, filePath)
	if err != nil {
		return false
	}

	// Helm uses slash-separated paths for matching.
	relPath = filepath.ToSlash(relPath)

	// Never ignore the base directory itself.
	if relPath == "" || relPath == "." || relPath == "./" {
		return false
	}

	for _, pattern := range *i {
		match, negate, mustDir, compileErr := compilePattern(pattern)
		if compileErr != nil || match == nil {
			continue
		}

		if negate {
			// This mirrors Helm's (intentionally) unusual negation handling.
			if mustDir && !isDir {
				return true
			}
			if !match(relPath) {
				return true
			}
			continue
		}

		if mustDir && !isDir {
			continue
		}

		if match(relPath) {
			return true
		}
	}

	return false
}

// compilePattern parses a single .helmignore rule and returns its match
// function plus its negation and directory-only flags. The semantics follow
// Helm's pkg/ignore rules.
func compilePattern(pattern string) (match func(string) bool, negate, mustDir bool, err error) {
	rule := strings.TrimSpace(pattern)

	// Ignore blank lines and comments.
	if rule == "" || strings.HasPrefix(rule, "#") {
		return nil, false, false, nil
	}

	// Helm rejects recursive globs.
	if strings.Contains(rule, "**") {
		return nil, false, false, errors.New("double-star (**) syntax is not supported")
	}

	// Validate the pattern before stripping modifiers.
	if _, matchErr := filepath.Match(rule, "abc"); matchErr != nil {
		return nil, false, false, matchErr
	}

	if strings.HasPrefix(rule, "!") {
		negate = true
		rule = rule[1:]
	}

	if strings.HasSuffix(rule, "/") {
		mustDir = true
		rule = strings.TrimSuffix(rule, "/")
	}

	switch {
	case strings.HasPrefix(rule, "/"):
		// Root-anchored: match against the full relative path.
		anchored := rule[1:]
		match = func(path string) bool {
			ok, matchErr := filepath.Match(anchored, path)
			return matchErr == nil && ok
		}
	case strings.Contains(rule, "/"):
		// Contains a path separator: match against the full relative path.
		full := rule
		match = func(path string) bool {
			ok, matchErr := filepath.Match(full, path)
			return matchErr == nil && ok
		}
	default:
		// No separator: match the base name on every directory level.
		base := rule
		match = func(path string) bool {
			ok, matchErr := filepath.Match(base, filepath.Base(path))
			return matchErr == nil && ok
		}
	}

	return match, negate, mustDir, nil
}
