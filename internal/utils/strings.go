package utils

import (
	"strings"
)

// Trimmed trims surrounding whitespace from every element and drops empty
// entries. The returned slice is never nil.
func Trimmed(values []string) []string {
	trimmed := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		trimmed = append(trimmed, value)
	}
	return trimmed
}
