package suggest

import (
	"bytes"
	"encoding/json"
	"sort"
	"strconv"
	"strings"
	"unicode"
)

const (
	// MinWordLength keeps short words such as "id" from listing every field.
	MinWordLength = 3
	maxMatches    = 50
	// templateMarker marks values that are unresolved {{variables}}, not real data.
	templateMarker = "{{"
)

// NewIndex collects the scalar field values of sources. When a field value
// appears more than once the first occurrence is kept, so pass the most
// relevant (newest) sources first.
func NewIndex(sources []Source) *Index {
	index := &Index{}
	seen := make(map[FieldValue]bool)
	for _, source := range sources {
		var document any
		decoder := json.NewDecoder(bytes.NewReader([]byte(source.Body)))
		decoder.UseNumber()
		if err := decoder.Decode(&document); err != nil {
			continue
		}
		collectFieldValues(document, func(field, value string) {
			key := FieldValue{Field: field, Value: value}
			if seen[key] {
				return
			}
			seen[key] = true
			index.values = append(index.values, FieldValue{Field: field, Value: value, Origin: source.Origin})
		})
	}
	return index
}

// Match returns the values of fields whose name starts with word, ignoring case,
// underscores and dashes, so "persona_" matches both persona_id and personaId.
func (i *Index) Match(word string) []FieldValue {
	prefix := normalizeFieldName(word)
	if len(prefix) < MinWordLength {
		return nil
	}
	var matches []FieldValue
	for _, value := range i.values {
		if strings.HasPrefix(normalizeFieldName(value.Field), prefix) {
			matches = append(matches, value)
			if len(matches) == maxMatches {
				break
			}
		}
	}
	return matches
}

// TrailingWord returns the run of letters, digits, '_' and '-' that text ends with.
func TrailingWord(text string) string {
	start := len(text)
	for start > 0 {
		r := rune(text[start-1])
		if !isWordRune(r) {
			break
		}
		start--
	}
	return text[start:]
}

func isWordRune(r rune) bool {
	return r < unicode.MaxASCII && (unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' || r == '-')
}

func normalizeFieldName(name string) string {
	return strings.ToLower(strings.NewReplacer("_", "", "-", "").Replace(name))
}

func collectFieldValues(node any, add func(field, value string)) {
	switch typed := node.(type) {
	case map[string]any:
		fields := make([]string, 0, len(typed))
		for field := range typed {
			fields = append(fields, field)
		}
		sort.Strings(fields)
		for _, field := range fields {
			if value, ok := scalarText(typed[field]); ok {
				add(field, value)
				continue
			}
			collectFieldValues(typed[field], add)
		}
	case []any:
		for _, element := range typed {
			collectFieldValues(element, add)
		}
	}
}

func scalarText(value any) (string, bool) {
	switch typed := value.(type) {
	case string:
		return typed, typed != "" && !strings.Contains(typed, templateMarker)
	case json.Number:
		return typed.String(), true
	case bool:
		return strconv.FormatBool(typed), true
	}
	return "", false
}
