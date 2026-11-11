package suggest

import (
	"reflect"
	"strings"
	"testing"
)

const (
	responseOrigin = "response"
	requestOrigin  = "request"
)

func TestNewIndex_CollectsNestedScalarFields(t *testing.T) {
	index := NewIndex([]Source{{Origin: responseOrigin, Body: `{"user": {"persona_id": "p-1", "age": 42, "active": true, "tags": ["x"]}, "items": [{"persona_id": "p-2"}]}`}})

	got := index.Match("persona")
	want := []FieldValue{
		{Field: "persona_id", Value: "p-2", Origin: responseOrigin},
		{Field: "persona_id", Value: "p-1", Origin: responseOrigin},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Match = %+v, want %+v", got, want)
	}
	if got := index.Match("age"); len(got) != 1 || got[0].Value != "42" {
		t.Errorf("expected the number as text, got %+v", got)
	}
	if got := index.Match("active"); len(got) != 1 || got[0].Value != "true" {
		t.Errorf("expected the bool as text, got %+v", got)
	}
}

func TestNewIndex_KeepsTheFirstOccurrence(t *testing.T) {
	index := NewIndex([]Source{
		{Origin: responseOrigin, Body: `{"persona_id": "p-1"}`},
		{Origin: requestOrigin, Body: `{"persona_id": "p-1"}`},
		{Origin: requestOrigin, Body: `{"persona_id": "p-3"}`},
	})

	want := []FieldValue{
		{Field: "persona_id", Value: "p-1", Origin: responseOrigin},
		{Field: "persona_id", Value: "p-3", Origin: requestOrigin},
	}
	if got := index.Match("persona_"); !reflect.DeepEqual(got, want) {
		t.Errorf("Match = %+v, want %+v", got, want)
	}
}

func TestNewIndex_SkipsNonJSONEmptyAndTemplateValues(t *testing.T) {
	index := NewIndex([]Source{
		{Origin: responseOrigin, Body: "not json"},
		{Origin: requestOrigin, Body: `{"persona_id": "{{persona_id}}", "persona_name": "", "persona_note": null}`},
	})
	if got := index.Match("persona"); len(got) != 0 {
		t.Errorf("expected no values, got %+v", got)
	}
}

func TestMatch_IgnoresCaseUnderscoresAndDashes(t *testing.T) {
	index := NewIndex([]Source{{Origin: responseOrigin, Body: `{"personaId": "a", "PERSONA-NAME": "b", "claim_id": "c"}`}})

	got := index.Match("persona_")
	if len(got) != 2 {
		t.Errorf("expected personaId and PERSONA-NAME, got %+v", got)
	}
}

func TestMatch_ShortWordMatchesNothing(t *testing.T) {
	index := NewIndex([]Source{{Origin: responseOrigin, Body: `{"id": "1", "idempotency_key": "k"}`}})
	if got := index.Match("id"); got != nil {
		t.Errorf("expected nothing below %d characters, got %+v", MinWordLength, got)
	}
}

func TestMatch_CapsTheNumberOfMatches(t *testing.T) {
	values := make([]string, 0, maxMatches+5)
	for i := range maxMatches + 5 {
		values = append(values, `{"persona_id": "p-`+strings.Repeat("x", i+1)+`"}`)
	}
	sources := make([]Source, len(values))
	for i, body := range values {
		sources[i] = Source{Origin: responseOrigin, Body: body}
	}
	if got := NewIndex(sources).Match("persona"); len(got) != maxMatches {
		t.Errorf("expected %d matches, got %d", maxMatches, len(got))
	}
}

func TestTrailingWord(t *testing.T) {
	tests := map[string]string{
		`{"personaId": "persona_`: "persona_",
		"/users/claim-id":         "claim-id",
		"ends with space ":        "",
		"":                        "",
		"åäö_x":                   "_x",
	}
	for text, want := range tests {
		if got := TrailingWord(text); got != want {
			t.Errorf("TrailingWord(%q) = %q, want %q", text, got, want)
		}
	}
}
