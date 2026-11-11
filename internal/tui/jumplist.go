package tui

import (
	"fmt"
	"slices"

	tea "github.com/charmbracelet/bubbletea"
	"postOffice/internal/postman"
)

const (
	jumpBackKey = "ctrl+o"
	// jumpForwardKey stands in for Vim's ctrl+i, which terminals send as tab,
	// the split view key.
	jumpForwardKey = "ctrl+]"
	// maxJumps caps the jumplist, dropping the oldest jumps, like Vim's.
	maxJumps = 100
)

// jumpModes are the views the jumplist keys work from: the requests view and
// the views reached from it on the way to another collection.
var jumpModes = []ViewMode{ModeRequests, ModeResponse, ModeInfo, ModeCollections, ModeEnvironments, ModeVariables}

// jumpLocation is a place in the requests view: a collection (or the
// combined view), a folder in it and the item under the cursor.
type jumpLocation struct {
	collectionName string
	combined       bool
	breadcrumb     []string
	itemName       string
}

func (l jumpLocation) sameFolder(other jumpLocation) bool {
	return l.collectionName == other.collectionName && l.combined == other.combined && slices.Equal(l.breadcrumb, other.breadcrumb)
}

func (l jumpLocation) isSet() bool {
	return l.collectionName != ""
}

// currentJumpLocation is where the requests view is, unset outside it.
func (m Model) currentJumpLocation() jumpLocation {
	if m.mode != ModeRequests || m.collection == nil || m.viewingFavorites {
		return jumpLocation{}
	}
	location := jumpLocation{
		collectionName: m.collection.Info.Name,
		combined:       m.isViewingMergedCollection(),
		breadcrumb:     slices.Clone(m.breadcrumb),
	}
	if item, ok := m.selectedItem(); ok {
		location.itemName = item.Name
	}
	return location
}

// recordJump adds the requests view location left behind to the jumplist
// whenever a key moved the requests view to another folder or collection,
// also by way of other views (the collections list, a response), as Vim does
// for jumps. before is the model the key was handled on.
func (m Model) recordJump(before Model) Model {
	current := m.currentJumpLocation()
	if !current.isSet() {
		return m
	}
	previous := before.currentJumpLocation()
	if !previous.isSet() {
		previous = before.lastJumpLocation
	}
	m.lastJumpLocation = current
	if !previous.isSet() || previous.sameFolder(current) || m.jumping {
		m.jumping = false
		return m
	}
	m.jumps = append(m.jumps[:m.jumpIndex], previous)
	if len(m.jumps) > maxJumps {
		m.jumps = m.jumps[len(m.jumps)-maxJumps:]
	}
	m.jumpIndex = len(m.jumps)
	return m
}

func handleJumpBackKey(m Model) (Model, tea.Cmd) {
	if m.jumpIndex == len(m.jumps) {
		if current := m.currentJumpLocation(); current.isSet() {
			// Keep where we jump back from, so jumping forward returns here.
			m.jumps = append(m.jumps, current)
		} else if m.lastJumpLocation.isSet() {
			// From another view, the first jump back returns to where the
			// requests view was left.
			m.jumps = append(m.jumps, m.lastJumpLocation)
			return m.jumpTo(len(m.jumps) - 1)
		}
	}
	if m.jumpIndex == 0 {
		m.statusMessage = "Already at the oldest jump"
		return m, nil
	}
	return m.jumpTo(m.jumpIndex - 1)
}

func handleJumpForwardKey(m Model) (Model, tea.Cmd) {
	if m.jumpIndex >= len(m.jumps)-1 {
		m.statusMessage = "Already at the newest jump"
		return m, nil
	}
	return m.jumpTo(m.jumpIndex + 1)
}

func (m Model) jumpTo(index int) (Model, tea.Cmd) {
	location := m.jumps[index]
	if !m.showJumpCollection(location) {
		m.jumps = slices.Delete(m.jumps, index, index+1)
		m.jumpIndex = min(index, len(m.jumps))
		m.statusMessage = fmt.Sprintf("Collection %s is no longer loaded", location.collectionName)
		return m, nil
	}

	m.jumpIndex = index
	m.jumping = true
	m.viewingFavorites = false
	m = m.openRequestsFolder(location.breadcrumb)
	if i := slices.IndexFunc(m.currentItems, func(item postman.Item) bool { return item.Name == location.itemName }); i >= 0 {
		m.cursor = i
	}
	m.statusMessage = fmt.Sprintf("Jump %d/%d", index+1, len(m.jumps))
	return m, nil
}

func (m *Model) showJumpCollection(location jumpLocation) bool {
	if location.combined {
		merged := m.parser.MergedCollection()
		if merged == nil || merged.Info.Name != location.collectionName {
			return false
		}
		m.collection = merged
		m.mergedCollectionActive = true
		return true
	}
	collection, exists := m.parser.GetCollection(location.collectionName)
	if !exists {
		return false
	}
	m.collection = collection
	m.mergedCollectionActive = false
	return true
}
