package postman

import (
	"regexp"
	"strings"
)

type VariableSource struct {
	Key     string
	Value   string
	Choices []string
	Source  string
}

var variablePattern = regexp.MustCompile(`\{\{([^}]+)\}\}`)
var pathParamSegmentPattern = regexp.MustCompile(`:([\w]+)`)

// GetAllVariables returns the variables visible at breadcrumb, one per key, in
// precedence order: environment, then collection, then the folders on breadcrumb.
func GetAllVariables(collection *Collection, breadcrumb []string, environment *Environment) []VariableSource {
	var variables []VariableSource
	seen := make(map[string]bool)
	for _, variable := range VariableDefinitions(collection, breadcrumb, environment) {
		if !seen[variable.Key] {
			seen[variable.Key] = true
			variables = append(variables, variable)
		}
	}
	return variables
}

// VariableDefinitions returns every definition visible at breadcrumb in
// precedence order, including the ones a higher-precedence scope shadows.
func VariableDefinitions(collection *Collection, breadcrumb []string, environment *Environment) []VariableSource {
	var variables []VariableSource

	if environment != nil {
		for _, envVar := range environment.Values {
			if envVar.Enabled {
				variables = append(variables, VariableSource{
					Key:     envVar.Key,
					Value:   envVar.Value,
					Choices: envVar.Choices,
					Source:  "Environment: " + environment.Name,
				})
			}
		}
	}

	if collection == nil {
		return variables
	}
	for _, collVar := range collection.Variables {
		variables = append(variables, VariableSource{
			Key:     collVar.Key,
			Value:   collVar.Value,
			Choices: collVar.Choices,
			Source:  "Collection: " + collection.Info.Name,
		})
	}

	current := collection.Items
	for i, crumbName := range breadcrumb {
		for _, item := range current {
			if item.Name == crumbName && item.IsFolder() {
				folderPath := strings.Join(breadcrumb[:i+1], " / ")
				for _, folderVar := range item.Variables {
					variables = append(variables, VariableSource{
						Key:     folderVar.Key,
						Value:   folderVar.Value,
						Choices: folderVar.Choices,
						Source:  "Folder: " + folderPath,
					})
				}
				current = item.Items
				break
			}
		}
	}
	return variables
}

// OverrideVariables puts overrides ahead of variables and drops the
// variables they shadow, keeping the list free of duplicate keys (which
// ResolveVariables would otherwise resolve to the last occurrence).
func OverrideVariables(overrides, variables []VariableSource) []VariableSource {
	if len(overrides) == 0 {
		return variables
	}
	shadowed := make(map[string]bool, len(overrides))
	merged := make([]VariableSource, 0, len(overrides)+len(variables))
	for _, override := range overrides {
		shadowed[override.Key] = true
		merged = append(merged, override)
	}
	for _, variable := range variables {
		if !shadowed[variable.Key] {
			merged = append(merged, variable)
		}
	}
	return merged
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

// ResolveVariables replaces each {{key}} with its value, and each {{key[N]}}
// with a choice variable's N-th choice. Unknown references are left as is.
func ResolveVariables(text string, variables []VariableSource) string {
	variableMap := make(map[string]VariableSource)
	for _, v := range variables {
		variableMap[v.Key] = v
	}

	return variablePattern.ReplaceAllStringFunc(text, func(match string) string {
		key := strings.TrimSpace(match[2 : len(match)-2])
		if variable, exists := variableMap[key]; exists {
			return variable.Value
		}
		if choiceKey, index, ok := splitChoiceReference(key); ok {
			if variable, exists := variableMap[choiceKey]; exists && index < len(variable.Choices) {
				return variable.Choices[index]
			}
		}
		return match
	})
}
