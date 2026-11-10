package tui

import "postOffice/internal/postman"

const combinedRootEditMessage = "Open a collection first: the combined view's top level holds one folder per collection"

// owningCollectionName is the collection whose file holds folderPath. In the
// combined view every path starts at the group folder named after its
// collection.
func (m Model) owningCollectionName(folderPath []string) string {
	if m.isViewingMergedCollection() && len(folderPath) > 0 {
		return folderPath[0]
	}
	return m.collection.Info.Name
}

// markCollectionModified flags the collection holding folderPath as having
// unsaved changes.
func (m Model) markCollectionModified(folderPath []string) {
	m.modifiedCollections[m.owningCollectionName(folderPath)] = true
}

// saveOwningCollection writes the collection holding folderPath to its file.
func (m Model) saveOwningCollection(folderPath []string) error {
	m.syncMergedCollection()
	return m.parser.SaveCollection(m.owningCollectionName(folderPath))
}

// syncMergedCollection writes combined-view edits back to the collections it
// groups (see Parser.SyncMergedCollection).
func (m Model) syncMergedCollection() {
	if m.isViewingMergedCollection() {
		m.parser.SyncMergedCollection(m.collection)
	}
}

// isCombinedRoot reports whether folderPath is the combined view's top level,
// where only the collections' group folders belong.
func (m Model) isCombinedRoot(folderPath []string) bool {
	return m.isViewingMergedCollection() && len(folderPath) == 0
}

func (m Model) blockCombinedRootEdit() Model {
	m.statusMessage = combinedRootEditMessage
	m.statusIsError = true
	return m
}

// collectionScope is the real collection holding folderPath and the path
// inside it, which in the combined view drops the leading group folder.
func (m Model) collectionScope(folderPath []string) (*postman.Collection, []string) {
	if !m.isViewingMergedCollection() || len(folderPath) == 0 {
		return m.collection, folderPath
	}
	if collection, exists := m.parser.GetCollection(folderPath[0]); exists {
		return collection, folderPath[1:]
	}
	return m.collection, folderPath
}
