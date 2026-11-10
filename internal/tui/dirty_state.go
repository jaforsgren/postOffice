package tui

import (
	"slices"
	"strings"
)

const unsavedMarker = "* "

// hasUnsavedRequestsIn reports whether a request anywhere below folderPath
// has unsaved changes.
func (m Model) hasUnsavedRequestsIn(folderPath []string) bool {
	collection, path := m.collectionScope(folderPath)
	return m.hasUnsavedRequestsWithPrefix(m.getRequestIdentifierByPath(collection.Info.Name, path, ""))
}

// isCollectionUnsaved reports whether the collection or one of its requests
// has unsaved changes.
func (m Model) isCollectionUnsaved(name string) bool {
	return m.modifiedCollections[name] || m.hasUnsavedRequestsWithPrefix(m.getRequestIdentifierByPath(name, nil, ""))
}

func (m Model) hasUnsavedRequestsWithPrefix(prefix string) bool {
	for itemID, modified := range m.modifiedItems {
		if modified && strings.HasPrefix(itemID, prefix) {
			return true
		}
	}
	return false
}

// unsavedPrefix marks a list row whose folder, collection or environment
// has unsaved changes.
func (m Model) unsavedPrefix(index int) string {
	switch m.mode {
	case ModeCollections:
		if m.isCollectionUnsaved(m.items[index]) {
			return unsavedMarker
		}
	case ModeEnvironments:
		if m.modifiedEnvironments[m.items[index]] {
			return unsavedMarker
		}
	case ModeRequests:
		if index < len(m.currentItems) && m.currentItems[index].IsFolder() &&
			m.hasUnsavedRequestsIn(append(slices.Clone(m.folderPathForIndex(index)), m.currentItems[index].Name)) {
			return unsavedMarker
		}
	}
	return ""
}
