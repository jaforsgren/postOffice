package script

import (
	"encoding/json"
	"strings"

	"postOffice/internal/postman"
)

const (
	variableOpen  = "{{"
	variableClose = "}}"
)

// lexState is where in a script's source the resolver is, which decides how
// a {{variable}} found there is written.
type lexState int

const (
	lexCode lexState = iota
	lexString
	lexLineComment
	lexBlockComment
)

// resolveScriptVariables replaces {{name}} references in code with the
// values ctx resolves them to at the moment the script runs (iteration data,
// then environment, then collection, as pm.variables.get does), so earlier
// scripts' pm.*.set calls are visible. A reference in code becomes a JS
// string literal (`const id = {{persona_id}}` declares a string); inside a
// string literal or comment the value is spliced in as text, escaped for the
// enclosing quote. Unknown references are left as written.
//
// Gotcha: a template literal's ${...} expression counts as part of the
// string, so a reference there is spliced in as text, not as a literal.
func resolveScriptVariables(code string, ctx *ExecutionContext) string {
	if !strings.Contains(code, variableOpen) {
		return code
	}
	variables := scriptVariableSources(ctx)

	var out strings.Builder
	state, quote := lexCode, byte(0)
	for i := 0; i < len(code); i++ {
		if strings.HasPrefix(code[i:], variableOpen) {
			if end := strings.Index(code[i+len(variableOpen):], variableClose); end >= 0 {
				reference := code[i : i+len(variableOpen)+end+len(variableClose)]
				if value, resolved := resolveReference(reference, variables); resolved {
					out.WriteString(writeScriptValue(value, state, quote))
					i += len(reference) - 1
					continue
				}
			}
		}

		c := code[i]
		out.WriteByte(c)
		switch state {
		case lexCode:
			switch {
			case c == '"' || c == '\'' || c == '`':
				state, quote = lexString, c
			case strings.HasPrefix(code[i:], "//"):
				state = lexLineComment
			case strings.HasPrefix(code[i:], "/*"):
				state = lexBlockComment
			}
		case lexString:
			if c == '\\' && i+1 < len(code) {
				i++
				out.WriteByte(code[i])
			} else if c == quote {
				state = lexCode
			}
		case lexLineComment:
			if c == '\n' {
				state = lexCode
			}
		case lexBlockComment:
			if strings.HasPrefix(code[i:], "*/") {
				i++
				out.WriteByte(code[i])
				state = lexCode
			}
		}
	}
	return out.String()
}

func resolveReference(reference string, variables []postman.VariableSource) (string, bool) {
	value := postman.ResolveVariables(reference, variables)
	return value, value != reference
}

func writeScriptValue(value string, state lexState, quote byte) string {
	switch state {
	case lexCode:
		encoded, _ := json.Marshal(value)
		return string(encoded)
	case lexString:
		return escapeForQuote(value, quote)
	default:
		return value
	}
}

func escapeForQuote(value string, quote byte) string {
	replacer := strings.NewReplacer(`\`, `\\`, string(quote), `\`+string(quote), "\n", `\n`, "\r", `\r`)
	return replacer.Replace(value)
}

// scriptVariableSources lists ctx's variables in pm.variables.get's
// precedence, without duplicate keys.
func scriptVariableSources(ctx *ExecutionContext) []postman.VariableSource {
	var data, environment, collection []postman.VariableSource
	for key := range ctx.Iteration.Data {
		if value, ok := ctx.Iteration.Data.value(key); ok {
			data = append(data, postman.VariableSource{Key: key, Value: value})
		}
	}
	for _, variable := range ctx.EnvironmentVars {
		environment = append(environment, postman.VariableSource{Key: variable.Key, Value: variable.Value, Choices: variable.Choices})
	}
	for _, variable := range ctx.CollectionVars {
		collection = append(collection, postman.VariableSource{Key: variable.Key, Value: variable.Value, Choices: variable.Choices})
	}
	return postman.OverrideVariables(data, postman.OverrideVariables(firstPerKey(environment), firstPerKey(collection)))
}

// firstPerKey keeps each key's first occurrence, the one the pm getters return.
func firstPerKey(variables []postman.VariableSource) []postman.VariableSource {
	seen := make(map[string]bool, len(variables))
	var unique []postman.VariableSource
	for _, variable := range variables {
		if !seen[variable.Key] {
			seen[variable.Key] = true
			unique = append(unique, variable)
		}
	}
	return unique
}
