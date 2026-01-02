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
