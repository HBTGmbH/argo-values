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
