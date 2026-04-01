package config

import (
	"fmt"
	"os"
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
// and reconstructs a nested YAML-like map. Keys are split on "_" and lowercased to build the hierarchy.
func UnflattenYAML(flat map[string]string, sanitized bool) map[string]interface{} {
	result := make(map[string]interface{})

	for flatKey, value := range flat {
		if sanitized {
			flatKey = UnSanitizeKey(flatKey)
		}
		parts := strings.Split(flatKey, ".")
		current := result

		for i, part := range parts {
			if i == len(parts)-1 {
				// Last part: set the value
				current[part] = value
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
