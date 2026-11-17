package postman

import "strings"

// dynamicVariablePrefix marks Postman's generated variables ({{$guid}}, ...),
// which have no stored value to edit.
const dynamicVariablePrefix = "$"

// ReferencedVariables lists the {{name}} variables req uses in its URL,
// headers and body, in order of first use, leaving out dynamic variables. A
// {{name[N]}} choice reference counts as a use of name.
func ReferencedVariables(req *Request) []string {
	return referencedNames(requestTexts(req))
}

// ReferencedItemVariables lists the variables item's request uses, then the
// ones only its own scripts use, which are resolved when the scripts run.
func ReferencedItemVariables(item Item) []string {
	texts := requestTexts(item.Request)
	for _, event := range item.Events {
		texts = append(texts, event.Script.Exec...)
	}
	return referencedNames(texts)
}

func requestTexts(req *Request) []string {
	if req == nil {
		return nil
	}
	texts := []string{req.URL.Raw}
	for _, header := range req.Header {
		texts = append(texts, header.Key, header.Value)
	}
	if req.Body != nil {
		texts = append(texts, req.Body.Raw)
		for _, param := range req.Body.FormData {
			texts = append(texts, param.Key, param.Value)
		}
	}
	return texts
}

func referencedNames(texts []string) []string {
	var names []string
	seen := make(map[string]bool)
	for _, text := range texts {
		for _, match := range variablePattern.FindAllStringSubmatch(text, -1) {
			name := referencedVariableKey(strings.TrimSpace(match[1]))
			if seen[name] || strings.HasPrefix(name, dynamicVariablePrefix) {
				continue
			}
			seen[name] = true
			names = append(names, name)
		}
	}
	return names
}

type VariableScope int

const (
	VariableScopeEnvironment VariableScope = iota
	VariableScopeCollection
	VariableScopeFolder
)

// VariableOwner is where a variable's value is stored: the environment,
// collection or folder (as a " / " joined path) that defines it.
type VariableOwner struct {
	Scope VariableScope
	Name  string
}

// SetVariable stores value (and its choices and their notes) where GetAllVariables resolves
// key from, so the new value is the one requests use. environments are in
// precedence order: the first one defining key wins, as in
// Parser.MergedEnvironment. A key defined nowhere becomes a new collection
// variable.
func SetVariable(collection *Collection, folderPath []string, environments []*Environment, key string, value VariableValue) VariableOwner {
	for _, environment := range environments {
		for i := range environment.Values {
			if environment.Values[i].Enabled && environment.Values[i].Key == key {
				environment.Values[i].Value = value.Value
				environment.Values[i].Choices = value.Choices
				environment.Values[i].ChoiceNotes = value.ChoiceNotes
				return VariableOwner{Scope: VariableScopeEnvironment, Name: environment.Name}
			}
		}
	}

	collectionOwner := VariableOwner{Scope: VariableScopeCollection, Name: collection.Info.Name}
	for i := range collection.Variables {
		if collection.Variables[i].Key == key {
			setValue(&collection.Variables[i], value)
			return collectionOwner
		}
	}

	if folderName, found := setFolderVariable(collection.Items, folderPath, key, value); found {
		return VariableOwner{Scope: VariableScopeFolder, Name: folderName}
	}

	collection.Variables = append(collection.Variables, Variable{Key: key, Value: value.Value, Choices: value.Choices, ChoiceNotes: value.ChoiceNotes})
	return collectionOwner
}

func setValue(variable *Variable, value VariableValue) {
	variable.Value = value.Value
	variable.Choices = value.Choices
	variable.ChoiceNotes = value.ChoiceNotes
}

// setFolderVariable walks folderPath from the root, as GetAllVariables does,
// and sets key in the first folder defining it.
func setFolderVariable(items []Item, folderPath []string, key string, value VariableValue) (string, bool) {
	current := items
	for depth, folderName := range folderPath {
		folder := findFolder(current, folderName)
		if folder == nil {
			return "", false
		}
		for i := range folder.Variables {
			if folder.Variables[i].Key == key {
				setValue(&folder.Variables[i], value)
				return strings.Join(folderPath[:depth+1], " / "), true
			}
		}
		current = folder.Items
	}
	return "", false
}

func findFolder(items []Item, name string) *Item {
	for i := range items {
		if items[i].Name == name && items[i].IsFolder() {
			return &items[i]
		}
	}
	return nil
}

// UnresolvedVariables lists, as {{key}}, the variables req uses that
// variables don't define.
func UnresolvedVariables(req *Request, variables []VariableSource) []string {
	defined := make(map[string]bool, len(variables))
	for _, variable := range variables {
		defined[variable.Key] = true
	}
	var keys []string
	for _, key := range ReferencedVariables(req) {
		if !defined[key] {
			keys = append(keys, "{{"+key+"}}")
		}
	}
	return keys
}
