package utils

import "k8s.io/utils/strings/slices"

func NonEmpty(values []string) []string {
	return slices.Filter([]string{}, values, func(s string) bool {
		return s != ""
	})
}
