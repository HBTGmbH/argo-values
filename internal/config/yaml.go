package config

import (
	"fmt"
	"os"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

func ParseYAML(key, value string) map[string]interface{} {
	var result map[string]interface{}
	err := yaml.Unmarshal([]byte(value), &result)
	if err != nil {
		result = map[string]interface{}{key: value}
	}
	return result
}

func SanitizeKey(key string) string {
	return strings.ReplaceAll(strings.ReplaceAll(strings.ToUpper(key), ".", "_"), "-", "_")
}

func UnSanitizeKey(key string) string {
	return strings.ReplaceAll(strings.ToLower(key), "_", ".")
}

func FlattenYAML(data map[string]interface{}, prefix string, result map[string]string) {
	for key, value := range data {
		newKey := prefix + SanitizeKey(key)

		switch v := value.(type) {
		case map[string]interface{}:
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
// UnSanitizeKey (lowercased, "_" replaced by "."); otherwise the key is used as-is. In both cases
// the resulting key is split on "." to build the hierarchy.
//
// Entries are inserted in a deterministic order: shallower paths first, then lexicographically by
// transformed key and finally by original key. This makes deeper paths win over conflicting
// shallower leaf values (e.g. "a.b" overwrites the leaf "a"), independent of map iteration order.
func UnflattenYAML(flat map[string]string, sanitized bool) map[string]interface{} {
	type entry struct {
		originalKey string
		key         string
		depth       int
		value       string
	}

	entries := make([]entry, 0, len(flat))
	for originalKey, value := range flat {
		key := originalKey
		if sanitized {
			key = UnSanitizeKey(originalKey)
		}
		entries = append(entries, entry{
			originalKey: originalKey,
			key:         key,
			depth:       strings.Count(key, ".") + 1,
			value:       value,
		})
	}

	sort.Slice(entries, func(i, j int) bool {
		if entries[i].depth != entries[j].depth {
			return entries[i].depth < entries[j].depth
		}
		if entries[i].key != entries[j].key {
			return entries[i].key < entries[j].key
		}
		return entries[i].originalKey < entries[j].originalKey
	})

	result := make(map[string]interface{})

	for _, e := range entries {
		parts := strings.Split(e.key, ".")
		current := result

		for i, part := range parts {
			if i == len(parts)-1 {
				// Last part: set the value
				current[part] = e.value
			} else {
				// Intermediate part: ensure a nested map exists
				if existing, ok := current[part]; ok {
					if nestedMap, ok := existing.(map[string]interface{}); ok {
						current = nestedMap
					} else {
						// Conflict: a leaf value already exists at this key; overwrite with a map
						newMap := make(map[string]interface{})
						current[part] = newMap
						current = newMap
					}
				} else {
					newMap := make(map[string]interface{})
					current[part] = newMap
					current = newMap
				}
			}
		}
	}

	return result
}

func MergeYAML(a, b map[string]interface{}) map[string]interface{} {
	result := make(map[string]interface{}, len(a))
	for k, v := range a {
		result[k] = v
	}
	for k, v := range b {
		if v, ok := v.(map[string]interface{}); ok {
			if bv, ok := result[k]; ok {
				if bv, ok := bv.(map[string]interface{}); ok {
					result[k] = MergeYAML(bv, v)
					continue
				}
			}
		}
		result[k] = v
	}
	return result
}

func SaveYAML(yamlData map[string]interface{}, file string) error {
	rawYaml, err := yaml.Marshal(&yamlData)
	if err != nil {
		return err
	}
	err = os.WriteFile(file, rawYaml, 0644)
	if err != nil {
		return err
	}
	return nil
}

func SaveTemp(yamlData map[string]interface{}) {
	tmpFile, _ := os.CreateTemp("", "argo-values-*.txt")
	rawYaml, _ := yaml.Marshal(&yamlData)
	if _, err := tmpFile.Write(rawYaml); err != nil {
		_ = tmpFile.Close()
	}
	_ = tmpFile.Close()
}
