package elements

import (
	"encoding/json"
	"log"
	"strconv"
	"strings"
)

// ExtractJSONPathSimple extracts a value from JSON using a simple dot notation path
// Examples: "user.name", "data.items.0.id", "response.token"
func ExtractJSONPathSimple(jsonStr, path string) string {
	if jsonStr == "" || path == "" {
		return ""
	}

	var data interface{}
	if err := json.Unmarshal([]byte(jsonStr), &data); err != nil {
		log.Printf("Error: Failed to parse JSON: %v", err)
		return ""
	}

	parts := strings.Split(path, ".")
	current := data

	for _, part := range parts {
		switch v := current.(type) {
		case map[string]interface{}:
			current = v[part]
		case []interface{}:
			// Try to parse part as array index
			idx, err := strconv.Atoi(part)
			if err == nil && idx >= 0 && idx < len(v) {
				current = v[idx]
			} else {
				return ""
			}
		default:
			return ""
		}

		if current == nil {
			return ""
		}
	}

	// Convert result to string
	switch v := current.(type) {
	case string:
		return v
	case float64:
		return strconv.FormatFloat(v, 'f', -1, 64)
	case bool:
		return strconv.FormatBool(v)
	case nil:
		return ""
	default:
		// For objects/arrays, return JSON representation
		bytes, err := json.Marshal(v)
		if err == nil {
			return string(bytes)
		}
		return ""
	}
}
