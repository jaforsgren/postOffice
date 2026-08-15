package postman

import (
	"encoding/json"
	"strings"
)

type CollectionFormat int

const (
	FormatPostman CollectionFormat = iota
	FormatOpenAPI
	FormatUnknown
)

func DetectFormat(data []byte) CollectionFormat {
	var raw map[string]interface{}
	if err := json.Unmarshal(data, &raw); err != nil {
		return FormatUnknown
	}

	if _, hasOpenAPI := raw["openapi"]; hasOpenAPI {
		return FormatOpenAPI
	}
	if _, hasSwagger := raw["swagger"]; hasSwagger {
		return FormatOpenAPI
	}

	if info, ok := raw["info"].(map[string]interface{}); ok {
		if schema, ok := info["schema"].(string); ok {
			if strings.Contains(schema, "postman") {
				return FormatPostman
			}
		}
	}

	if _, hasItem := raw["item"]; hasItem {
		return FormatPostman
	}

	if _, hasPaths := raw["paths"]; hasPaths {
		return FormatOpenAPI
	}

	return FormatUnknown
}

// IsEnvironmentFormat reports whether data looks like a Postman environment
// file: it has a "values" array and no "item" array (which would make it a collection).
func IsEnvironmentFormat(data []byte) bool {
	var raw map[string]interface{}
	if err := json.Unmarshal(data, &raw); err != nil {
		return false
	}

	_, hasValues := raw["values"]
	_, hasItem := raw["item"]
	return hasValues && !hasItem
}
