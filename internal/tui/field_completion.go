package tui

import (
	"sort"
	"strings"

	"postOffice/internal/logger"
	"postOffice/internal/postman"
	"postOffice/internal/suggest"
)

const (
	variableOpen  = "{{"
	variableClose = "}}"

	responseOrigin      = "response"
	sentRequestOrigin   = "sent"
	savedResponseOrigin = "saved response"
	savedRequestOrigin  = "request"
	originSeparator     = " · "
)

type completionKind int

const (
	completionVariables completionKind = iota
	completionFieldValues
)

// completionItem is one autocomplete suggestion: insert replaces the field from
// the completion's start, label is what the dropdown shows and detail where it comes from.
type completionItem struct {
	insert string
	label  string
	detail string
}

// fieldCompletion is the autocomplete dropdown of the field being edited.
// inserted is set once Tab has put items[cursor] into the field, so the next
// Tab replaces it with the following item instead of inserting again.
type fieldCompletion struct {
	kind     completionKind
	items    []completionItem
	cursor   int
	start    int
	inserted bool
}

func (c fieldCompletion) active() bool {
	return len(c.items) > 0
}

// cycle returns value with the next suggestion in place of the typed fragment
// (or of the suggestion the previous Tab inserted).
func (c *fieldCompletion) cycle(value string) string {
	if c.inserted {
		c.cursor = (c.cursor + 1) % len(c.items)
	}
	c.inserted = true
	return value[:min(c.start, len(value))] + c.items[c.cursor].insert
}

// startFieldCompletion clears the dropdown and indexes the field values of the
// request history, saved responses and saved request bodies for this edit.
func (m Model) startFieldCompletion() Model {
	m.fieldCompletion = fieldCompletion{}
	m.fieldValueIndex = suggest.NewIndex(m.fieldValueSources())
	return m
}

// completeField suggests variables inside an unclosed {{, and otherwise values of
// JSON fields whose name starts with the word the field ends with.
func (m Model) completeField(value string) fieldCompletion {
	if prefix, open := detectVarPrefix(value); open {
		return fieldCompletion{
			kind:  completionVariables,
			start: strings.LastIndex(value, variableOpen),
			items: m.variableCompletionItems(prefix),
		}
	}
	if m.fieldValueIndex == nil {
		return fieldCompletion{}
	}
	word := suggest.TrailingWord(value)
	var items []completionItem
	for _, match := range m.fieldValueIndex.Match(word) {
		if match.Value == word {
			continue
		}
		items = append(items, completionItem{insert: match.Value, label: match.Field + " = " + match.Value, detail: match.Origin})
	}
	return fieldCompletion{kind: completionFieldValues, start: len(value) - len(word), items: items}
}

// variableCompletionItems lists the variables visible to the edited request whose
// name starts with prefix, then those containing it, each with its scope and the
// definitions it shadows.
func (m Model) variableCompletionItems(prefix string) []completionItem {
	collection, folderPath := m.collectionScope(m.breadcrumb)
	definitions := make(map[string][]postman.VariableSource)
	var keys []string
	for _, definition := range postman.VariableDefinitions(collection, folderPath, m.environment) {
		if _, seen := definitions[definition.Key]; !seen {
			keys = append(keys, definition.Key)
		}
		definitions[definition.Key] = append(definitions[definition.Key], definition)
	}

	lowerPrefix := strings.ToLower(prefix)
	var startsWith, contains []completionItem
	for _, key := range keys {
		lowerKey := strings.ToLower(key)
		switch {
		case strings.HasPrefix(lowerKey, lowerPrefix):
			startsWith = append(startsWith, variableCompletionItem(definitions[key]))
		case strings.Contains(lowerKey, lowerPrefix):
			contains = append(contains, variableCompletionItem(definitions[key]))
		}
	}
	return append(startsWith, contains...)
}

// variableCompletionItem describes a variable from its definitions in precedence
// order: the first is the one requests resolve, the rest are shadowed by it.
func variableCompletionItem(definitions []postman.VariableSource) completionItem {
	visible := definitions[0]
	detail := visible.Source
	if len(definitions) > 1 {
		shadowed := make([]string, len(definitions)-1)
		for i, definition := range definitions[1:] {
			shadowed[i] = definition.Source + " = " + definition.Value
		}
		detail += originSeparator + "shadows " + strings.Join(shadowed, ", ")
	}
	label := variableOpen + visible.Key + variableClose
	if visible.Value != "" {
		label += " = " + visible.Value
	}
	return completionItem{insert: variableOpen + visible.Key + variableClose, label: label, detail: detail}
}

// fieldValueSources lists the bodies to suggest values from, newest first:
// history (responses received and requests sent), saved responses, then the
// request bodies saved in the collection.
func (m Model) fieldValueSources() []suggest.Source {
	var sources []suggest.Source
	for _, entry := range m.requestHistory.Entries() {
		when := entry.Snapshot.SavedAt.Format(timestampLayout)
		sources = append(sources,
			suggest.Source{Body: entry.Snapshot.Body, Origin: responseOrigin + originSeparator + entry.ItemName + originSeparator + when},
			suggest.Source{Body: entry.Snapshot.RequestBody, Origin: sentRequestOrigin + originSeparator + entry.ItemName + originSeparator + when},
		)
	}
	sources = append(sources, m.savedResponseSources()...)
	if m.collection != nil {
		sources = append(sources, savedRequestSources(m.collection.Items, nil)...)
	}
	return sources
}

func (m Model) savedResponseSources() []suggest.Source {
	store, err := m.parser.AllSavedResponses()
	if err != nil {
		// Suggestions are a convenience; an unreadable store only means fewer of them.
		logger.LogError("LoadSavedResponses", "", err)
		return nil
	}

	type savedResponse struct {
		itemID   string
		response postman.SavedResponse
	}
	var responses []savedResponse
	for itemID, saved := range store {
		for _, response := range saved {
			responses = append(responses, savedResponse{itemID: itemID, response: response})
		}
	}
	sort.SliceStable(responses, func(i, j int) bool {
		return responses[i].response.SavedAt.After(responses[j].response.SavedAt)
	})

	sources := make([]suggest.Source, 0, len(responses)*2)
	for _, saved := range responses {
		origin := savedResponseOrigin + originSeparator + saved.itemID + originSeparator + saved.response.SavedAt.Format(timestampLayout)
		sources = append(sources,
			suggest.Source{Body: saved.response.Body, Origin: origin},
			suggest.Source{Body: saved.response.RequestBody, Origin: origin},
		)
	}
	return sources
}

func savedRequestSources(items []postman.Item, folderPath []string) []suggest.Source {
	var sources []suggest.Source
	for _, item := range items {
		path := append(append([]string{}, folderPath...), item.Name)
		if item.IsFolder() {
			sources = append(sources, savedRequestSources(item.Items, path)...)
			continue
		}
		if item.Request != nil && item.Request.Body != nil {
			sources = append(sources, suggest.Source{Body: item.Request.Body.Raw, Origin: savedRequestOrigin + originSeparator + postman.JoinItemPath(path...)})
		}
	}
	return sources
}
