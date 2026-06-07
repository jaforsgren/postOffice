package postman

import (
	"regexp"
	"strings"
)

type VariableSource struct {
	Key    string
	Value  string
	Source string
}

var variablePattern = regexp.MustCompile(`\{\{([^}]+)\}\}`)
var pathParamSegmentPattern = regexp.MustCompile(`:([\w]+)`)

// GetAllVariables collects variables from environment, collection, and folder scopes in
// precedence order (environment overrides collection, collection overrides folders).
func GetAllVariables(collection *Collection, breadcrumb []string, environment *Environment) []VariableSource {
	var variables []VariableSource
	seen := make(map[string]bool)

	if environment != nil {
		for _, envVar := range environment.Values {
			if envVar.Enabled && !seen[envVar.Key] {
				variables = append(variables, VariableSource{
					Key:    envVar.Key,
					Value:  envVar.Value,
					Source: "Environment: " + environment.Name,
				})
				seen[envVar.Key] = true
			}
		}
	}

	if collection != nil {
		for _, collVar := range collection.Variables {
			if !seen[collVar.Key] {
				variables = append(variables, VariableSource{
					Key:    collVar.Key,
					Value:  collVar.Value,
					Source: "Collection: " + collection.Info.Name,
				})
				seen[collVar.Key] = true
			}
		}

		if len(breadcrumb) > 0 {
			current := collection.Items
			for i, crumbName := range breadcrumb {
				for _, item := range current {
					if item.Name == crumbName && item.IsFolder() {
						folderPath := strings.Join(breadcrumb[:i+1], " / ")
						for _, folderVar := range item.Variables {
							if !seen[folderVar.Key] {
								variables = append(variables, VariableSource{
									Key:    folderVar.Key,
									Value:  folderVar.Value,
									Source: "Folder: " + folderPath,
								})
								seen[folderVar.Key] = true
							}
						}
						current = item.Items
						break
					}
				}
			}
		}
	}

	return variables
}

// ExtractPathParams returns ordered path parameter names (without leading colon) from a URL.
// Checks URL.Path segments first, then scans URL.Raw as fallback.
func ExtractPathParams(url URL) []string {
	seen := make(map[string]bool)
	var params []string

	for _, segment := range url.Path {
		if strings.HasPrefix(segment, ":") {
			name := segment[1:]
			if name != "" && !seen[name] {
				params = append(params, name)
				seen[name] = true
			}
		}
	}

	if len(params) == 0 && url.Raw != "" {
		for _, match := range pathParamSegmentPattern.FindAllStringSubmatch(url.Raw, -1) {
			name := match[1]
			if !seen[name] {
				params = append(params, name)
				seen[name] = true
			}
		}
	}

	return params
}

func ResolveVariables(text string, variables []VariableSource) string {
	variableMap := make(map[string]string)
	for _, v := range variables {
		variableMap[v.Key] = v.Value
	}

	return variablePattern.ReplaceAllStringFunc(text, func(match string) string {
		key := strings.TrimSpace(match[2 : len(match)-2])
		if value, exists := variableMap[key]; exists {
			return value
		}
		return match
	})
}
