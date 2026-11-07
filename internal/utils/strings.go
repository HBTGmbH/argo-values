package utils

import (
	"strings"

	"k8s.io/utils/strings/slices"
)

func NonEmpty(values []string) []string {
	return slices.Filter([]string{}, values, func(s string) bool {
		return s != ""
	})
}

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
