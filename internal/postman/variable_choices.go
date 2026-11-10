package postman

import (
	"regexp"
	"slices"
	"strconv"
)

// choiceReferencePattern matches "{{key[N]}}" style references, which resolve
// to a choice variable's N-th (zero-based) choice instead of its chosen value.
var choiceReferencePattern = regexp.MustCompile(`^(.+)\[(\d+)\]$`)

// VariableValue is a variable's value and, for a choice variable, the values
// it can be switched between. Value is always the chosen one.
type VariableValue struct {
	Value   string
	Choices []string
}

// IsChoice reports whether the variable switches between choices.
func (v VariableValue) IsChoice() bool {
	return len(v.Choices) > 0
}

// ChoiceIndex is the position of the chosen value among the choices, or 0
// when it isn't one of them.
func (v VariableValue) ChoiceIndex() int {
	return max(slices.Index(v.Choices, v.Value), 0)
}

// WithChosen chooses choices[index].
func (v VariableValue) WithChosen(index int) VariableValue {
	if index < 0 || index >= len(v.Choices) {
		return v
	}
	return VariableValue{Value: v.Choices[index], Choices: slices.Clone(v.Choices)}
}

// WithChoice adds choice and chooses it. A plain variable's value becomes its
// first choice, so it can be switched back to.
func (v VariableValue) WithChoice(choice string) VariableValue {
	choices := slices.Clone(v.Choices)
	if len(choices) == 0 && v.Value != "" {
		choices = append(choices, v.Value)
	}
	if !slices.Contains(choices, choice) {
		choices = append(choices, choice)
	}
	return VariableValue{Value: choice, Choices: choices}
}

// WithoutChoice removes choices[index]. Removing the chosen value chooses the
// first remaining one, and a single remaining value makes the variable plain.
func (v VariableValue) WithoutChoice(index int) VariableValue {
	if index < 0 || index >= len(v.Choices) {
		return v
	}
	choices := slices.Delete(slices.Clone(v.Choices), index, index+1)
	value := v.Value
	if !slices.Contains(choices, value) && len(choices) > 0 {
		value = choices[0]
	}
	if len(choices) <= 1 {
		choices = nil
	}
	return VariableValue{Value: value, Choices: choices}
}

// splitChoiceReference splits a "key[N]" variable name into key and N.
func splitChoiceReference(name string) (string, int, bool) {
	match := choiceReferencePattern.FindStringSubmatch(name)
	if match == nil {
		return "", 0, false
	}
	index, err := strconv.Atoi(match[2])
	if err != nil {
		return "", 0, false
	}
	return match[1], index, true
}

// referencedVariableKey is the variable a {{name}} reference reads: the
// choice variable itself for a "key[N]" reference.
func referencedVariableKey(name string) string {
	if key, _, ok := splitChoiceReference(name); ok {
		return key
	}
	return name
}
