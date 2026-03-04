package utils

func GetStringFromMap(values map[string]interface{}, key string) string {
	if value, exists := values[key]; exists {
		if valueStr, ok := value.(string); ok {
			return valueStr
		}
	}
	return ""
}

func GetArrayFromMap(values map[string]interface{}, key string) []string {
	if value, exists := values[key]; exists {
		if valueArr, ok := value.([]interface{}); ok {
			valueStr := make([]string, len(valueArr))
			for i, v := range valueArr {
				if str, ok := v.(string); ok {
					valueStr[i] = str
				} else {
					return []string{}
				}
			}
			return valueStr
		}
	}
	return []string{}
}
