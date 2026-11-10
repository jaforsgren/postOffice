package postman

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"sync"
)

const (
	jsonTagSeparator = ","
	jsonTagSkip      = "-"
	emptyJSONObject  = "{}"
)

// unknownFields holds, as one JSON object, the keys of a Postman object that
// postOffice does not model (auth, saved responses, url.query, ...). Saving a
// collection writes them back unchanged instead of silently dropping them.
// It is a string rather than a map so the structs holding it stay comparable.
type unknownFields string

var knownKeysByType sync.Map

// decodeKeepingUnknown decodes data into known (a pointer to a method-free
// alias of the target struct) and keeps every key known does not model.
func decodeKeepingUnknown(data []byte, known any, unknown *unknownFields) error {
	if err := json.Unmarshal(data, known); err != nil {
		return err
	}
	var all map[string]json.RawMessage
	if err := json.Unmarshal(data, &all); err != nil {
		return err
	}
	for _, key := range knownKeys(reflect.TypeOf(known).Elem()) {
		delete(all, key)
	}
	if len(all) == 0 {
		*unknown = ""
		return nil
	}
	encoded, err := json.Marshal(all)
	if err != nil {
		return fmt.Errorf("failed to keep unmodelled fields: %w", err)
	}
	*unknown = unknownFields(encoded)
	return nil
}

// encodeWithUnknown encodes known and appends the kept unknown keys after the
// modelled ones, so a saved file keeps postOffice's field order.
func encodeWithUnknown(known any, unknown unknownFields) ([]byte, error) {
	encoded, err := json.Marshal(known)
	if err != nil || unknown == "" {
		return encoded, err
	}
	modelled := strings.TrimSuffix(string(encoded), "}")
	kept := strings.TrimPrefix(string(unknown), "{")
	if string(encoded) == emptyJSONObject {
		return []byte(modelled + kept), nil
	}
	return []byte(modelled + jsonTagSeparator + kept), nil
}

func knownKeys(structType reflect.Type) []string {
	if cached, ok := knownKeysByType.Load(structType); ok {
		return cached.([]string)
	}
	var keys []string
	for i := 0; i < structType.NumField(); i++ {
		field := structType.Field(i)
		if !field.IsExported() {
			continue
		}
		name, _, _ := strings.Cut(field.Tag.Get("json"), jsonTagSeparator)
		switch name {
		case jsonTagSkip:
			continue
		case "":
			name = field.Name
		}
		keys = append(keys, name)
	}
	knownKeysByType.Store(structType, keys)
	return keys
}
