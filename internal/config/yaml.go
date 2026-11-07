package config

import (
	"argo-values/internal/logger"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

// maxArrayIndex bounds array indices reconstructed by UnflattenYAML. Larger
// indices are treated as literal map keys to avoid allocating huge sparse slices.
const maxArrayIndex = 10000

var (
	arraySectionPattern = regexp.MustCompile(`^([^\[\]]+)((?:\[\d+\])+)$`)
	arrayIndexPattern   = regexp.MustCompile(`\[(\d+)\]`)
	intScalarPattern    = regexp.MustCompile(`^[+-]?(0|[1-9][0-9]*)$`)
	floatScalarPattern  = regexp.MustCompile(`^[+-]?(0|[1-9][0-9]*)(\.[0-9]+)?([eE][+-]?[0-9]+)?$`)
)

// CoerceScalars recursively converts string leaves that are unambiguous YAML
// scalars into their typed representation (bool, int64, float64, nil).
// Empty strings, strings with leading zeros ("0123"), hex/underscore numbers
// ("0x10", "1_000"), timestamps, quoted strings and non-scalar values
// (maps/sequences encoded as strings) are kept unchanged.
func CoerceScalars(data map[string]any) map[string]any {
	result := make(map[string]any, len(data))
	for key, value := range data {
		result[key] = coerceScalarValue(value)
	}
	return result
}

func coerceScalarValue(value any) any {
	switch v := value.(type) {
	case map[string]any:
		return CoerceScalars(v)
	case []any:
		list := make([]any, len(v))
		for i, item := range v {
			list[i] = coerceScalarValue(item)
		}
		return list
	case string:
		return coerceScalarString(v)
	default:
		return value
	}
}

func coerceScalarString(value string) any {
	if value == "" {
		return value
	}
	if strings.EqualFold(value, "true") {
		return true
	}
	if strings.EqualFold(value, "false") {
		return false
	}
	if strings.EqualFold(value, "null") || value == "~" {
		return nil
	}
	if intScalarPattern.MatchString(value) {
		if number, err := strconv.ParseInt(value, 10, 64); err == nil {
			return number
		}
		return value
	}
	if floatScalarPattern.MatchString(value) {
		if number, err := strconv.ParseFloat(value, 64); err == nil {
			return number
		}
		return value
	}
	return value
}

// pathStep is a single step in an unflattened path: either a map key or an
// array index. Exactly one of key/isIndex is meaningful.
type pathStep struct {
	key     string
	index   int
	isIndex bool
}

// ParseYAML parses value as a YAML map and uses key as the label for the
// result. If value is not a valid YAML map (error or nil result), the key is
// preserved and the original value is kept as a string. A warning is logged if
// the value looks like structured YAML but cannot be parsed, to surface silent
// misconfiguration without warning for plain scalars such as URLs.
func ParseYAML(key, value string) map[string]any {
	var result map[string]any
	if err := yaml.Unmarshal([]byte(value), &result); err != nil {
		if looksLikeStructuredYAML(value) {
			logger.Warnf("failed to parse value for key %q as YAML, using it as a string: %v", key, err)
		}
		return map[string]any{key: value}
	}
	if result == nil {
		return map[string]any{key: value}
	}
	return result
}

// ParseEnv treats key as an environment variable name and value as its literal
// string value. Unlike ParseYAML, the value is never interpreted as YAML.
func ParseEnv(key, value string) map[string]any {
	return map[string]any{key: value}
}

// looksLikeStructuredYAML reports whether value appears to contain structured
// YAML (a multi-line document or a flow map/sequence). Plain scalars such as
// URLs or version strings are not considered structured.
func looksLikeStructuredYAML(value string) bool {
	trimmed := strings.TrimSpace(value)
	if strings.Contains(trimmed, "\n") {
		return true
	}
	return strings.HasPrefix(trimmed, "{") || strings.HasPrefix(trimmed, "[")
}

func SanitizeKey(key string) string {
	return strings.ReplaceAll(strings.ReplaceAll(strings.ToUpper(key), ".", "_"), "-", "_")
}

func UnSanitizeKey(key string) string {
	return strings.ReplaceAll(strings.ToLower(key), "_", ".")
}

func FlattenYAML(data map[string]any, prefix string, result map[string]string) {
	for key, value := range data {
		newKey := prefix + SanitizeKey(key)

		switch v := value.(type) {
		case map[string]any:
			// If the value is a nested map, recurse.
			FlattenYAML(v, newKey+"_", result)
		case string, int, bool, float64:
			// If the value is a leaf node, add it to the result.
			result[newKey] = fmt.Sprintf("%v", value)
		}
	}
}

// UnflattenYAML takes a map of flattened key-value pairs (e.g., "RESOURCES_REQUESTS_CPU" -> "123m")
// and reconstructs a nested YAML-like map. If sanitized is true, each key is first passed through
// UnSanitizeKey (lowercased, "_" replaced by "."); otherwise the key is used as-is. The resulting
// key is split on "." and each section is parsed into map and array steps.
//
// Array sections such as "env[0]", "ports[0].name" or "matrix[0][1]" are reconstructed as
// []interface{} values. Sections that do not match the supported syntax (e.g. "foo[bar]", "foo[0]x"
// or a leading "[0]") remain literal map keys. Sparse arrays are padded with nil entries, so only
// "env[1]" yields []interface{}{nil, "B"}. Indices above maxArrayIndex (10000) are treated as
// literal map keys to protect against memory exhaustion from huge sparse allocations.
//
// Entries are inserted in a deterministic order: shallower paths first (depth is the number of
// parsed steps, including array indices), then lexicographically by transformed key and finally by
// original key. This makes deeper paths win over conflicting shallower values: a leaf is replaced
// by a map or list when a deeper path requires it (e.g. "a.b" overwrites the leaf "a", "env[0]"
// overwrites the leaf "env"), independent of map iteration order.
func UnflattenYAML(flat map[string]string, sanitized bool) map[string]any {
	type entry struct {
		originalKey string
		key         string
		steps       []pathStep
		value       string
	}

	entries := make([]entry, 0, len(flat))
	for originalKey, value := range flat {
		key := originalKey
		if sanitized {
			key = UnSanitizeKey(originalKey)
		}
		steps := parsePath(key)
		entries = append(entries, entry{
			originalKey: originalKey,
			key:         key,
			steps:       steps,
			value:       value,
		})
	}

	sort.Slice(entries, func(i, j int) bool {
		if len(entries[i].steps) != len(entries[j].steps) {
			return len(entries[i].steps) < len(entries[j].steps)
		}
		if entries[i].key != entries[j].key {
			return entries[i].key < entries[j].key
		}
		return entries[i].originalKey < entries[j].originalKey
	})

	result := make(map[string]any)

	for _, e := range entries {
		result = setPath(result, e.steps, e.value).(map[string]any)
	}

	return result
}

// parsePath splits a dotted key into map and array steps. Each dot-separated
// section is either a plain map key or a key followed by one or more "[n]"
// indices; everything else is kept as a single literal map key.
func parsePath(key string) []pathStep {
	sections := strings.Split(key, ".")
	steps := make([]pathStep, 0, len(sections))
	for _, section := range sections {
		steps = append(steps, parseSection(section)...)
	}
	return steps
}

func parseSection(section string) []pathStep {
	match := arraySectionPattern.FindStringSubmatch(section)
	if match == nil {
		return []pathStep{{key: section}}
	}

	submatches := arrayIndexPattern.FindAllStringSubmatch(match[2], -1)
	steps := make([]pathStep, 0, len(submatches)+1)
	steps = append(steps, pathStep{key: match[1]})
	for _, submatch := range submatches {
		index, err := strconv.Atoi(submatch[1])
		if err != nil || index > maxArrayIndex {
			// Do not expand untrusted or oversized indices; keep the section literal.
			return []pathStep{{key: section}}
		}
		steps = append(steps, pathStep{index: index, isIndex: true})
	}
	return steps
}

// setPath inserts value at the given steps and returns the resulting container.
// Intermediate values are created as maps or slices, replacing an existing leaf
// or a conflicting container type (the deeper path wins). Missing slice entries
// up to the requested index are padded with nil.
func setPath(node any, steps []pathStep, value string) any {
	if len(steps) == 0 {
		return value
	}

	step := steps[0]
	if step.isIndex {
		list, ok := node.([]any)
		if !ok {
			list = nil
		}
		for len(list) <= step.index {
			list = append(list, nil)
		}
		list[step.index] = setPath(list[step.index], steps[1:], value)
		return list
	}

	nestedMap, ok := node.(map[string]any)
	if !ok {
		nestedMap = make(map[string]any)
	}
	nestedMap[step.key] = setPath(nestedMap[step.key], steps[1:], value)
	return nestedMap
}

func MergeYAML(a, b map[string]any) map[string]any {
	result := make(map[string]any, len(a))
	maps.Copy(result, a)
	for k, v := range b {
		if v, ok := v.(map[string]any); ok {
			if bv, ok := result[k]; ok {
				if bv, ok := bv.(map[string]any); ok {
					result[k] = MergeYAML(bv, v)
					continue
				}
			}
		}
		result[k] = v
	}
	return result
}

func SaveYAML(yamlData map[string]any, file string) error {
	rawYaml, err := yaml.Marshal(&yamlData)
	if err != nil {
		return err
	}

	tmpFile, err := os.CreateTemp(filepath.Dir(file), ".argo-values-*.yaml")
	if err != nil {
		return err
	}
	tmpName := tmpFile.Name()
	defer os.Remove(tmpName)

	if err := tmpFile.Chmod(0600); err != nil {
		_ = tmpFile.Close()
		return err
	}
	if _, err := tmpFile.Write(rawYaml); err != nil {
		_ = tmpFile.Close()
		return err
	}
	if err := tmpFile.Close(); err != nil {
		return err
	}

	info, err := os.Lstat(file)
	if err == nil {
		if !info.Mode().IsRegular() {
			return fmt.Errorf("refusing to overwrite non-regular file %s", file)
		}
	} else if !os.IsNotExist(err) {
		return err
	}

	return os.Rename(tmpName, file)
}
