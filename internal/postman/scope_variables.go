package postman

import (
	"errors"
	"slices"
)

// ErrScopeNotFound is returned when a folder path names no folder.
var ErrScopeNotFound = errors.New("folder not found")

// ScopeVariables returns the variables defined on the folder at folderPath
// itself, or on the collection for an empty folderPath. Unlike
// GetAllVariables it leaves out everything inherited.
func ScopeVariables(collection *Collection, folderPath []string) ([]Variable, error) {
	variables, err := scopeVariableList(collection, folderPath)
	if err != nil {
		return nil, err
	}
	return *variables, nil
}

// SetScopeVariable stores value (with its choices and their notes) under key
// on the folder at folderPath, or on the collection for an empty folderPath,
// adding the variable when that scope doesn't define it yet.
func SetScopeVariable(collection *Collection, folderPath []string, key string, value VariableValue) error {
	variables, err := scopeVariableList(collection, folderPath)
	if err != nil {
		return err
	}
	if index := slices.IndexFunc(*variables, func(v Variable) bool { return v.Key == key }); index >= 0 {
		setValue(&(*variables)[index], value)
		return nil
	}
	*variables = append(*variables, Variable{Key: key, Value: value.Value, Choices: value.Choices, ChoiceNotes: value.ChoiceNotes})
	return nil
}

// DeleteScopeVariable removes key from the folder at folderPath, or from the
// collection for an empty folderPath.
func DeleteScopeVariable(collection *Collection, folderPath []string, key string) error {
	variables, err := scopeVariableList(collection, folderPath)
	if err != nil {
		return err
	}
	*variables = slices.DeleteFunc(*variables, func(v Variable) bool { return v.Key == key })
	return nil
}

func scopeVariableList(collection *Collection, folderPath []string) (*[]Variable, error) {
	if len(folderPath) == 0 {
		return &collection.Variables, nil
	}
	items := collection.Items
	var folder *Item
	for _, name := range folderPath {
		if folder = findFolder(items, name); folder == nil {
			return nil, ErrScopeNotFound
		}
		items = folder.Items
	}
	return &folder.Variables, nil
}
