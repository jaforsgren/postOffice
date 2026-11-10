package tui

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
)

const (
	cancelRequestKey = "ctrl+x"
	statusCancelled  = "Cancelled"
)

// handleCancelRequestKey cancels the selected request while it is being
// sent, or the only request being sent when the selection isn't one.
func handleCancelRequestKey(m Model) (Model, tea.Cmd) {
	itemID, ok := m.cancellableRequestID()
	if !ok {
		if len(m.inFlightRequests) == 0 {
			m.statusMessage = "No request is being sent"
		} else {
			m.statusMessage = "Select the request to cancel"
		}
		return m, nil
	}
	m.inFlightRequests[itemID]()
	m.statusMessage = fmt.Sprintf("Cancelling %s...", itemID)
	return m, nil
}

func (m Model) cancellableRequestID() (string, bool) {
	if item, ok := m.selectedItem(); ok && m.mode == ModeRequests && item.IsRequest() {
		if itemID := m.requestItemID(m.folderPathForCursor(), item.Name); m.inFlightRequests[itemID] != nil {
			return itemID, true
		}
	}
	if len(m.inFlightRequests) == 1 {
		for itemID := range m.inFlightRequests {
			return itemID, true
		}
	}
	return "", false
}

// forgetInFlightRequest releases a finished request's cancel function.
func (m Model) forgetInFlightRequest(itemID string) Model {
	if cancel, ok := m.inFlightRequests[itemID]; ok {
		cancel()
		delete(m.inFlightRequests, itemID)
	}
	return m
}

// completeCancelledRequest marks a cancelled request in the list. It isn't
// recorded in history and its script variable changes aren't saved, since
// the request never completed.
func (m Model) completeCancelledRequest(msg RequestCompleteMsg) Model {
	m = m.forgetInFlightRequest(msg.ItemID)
	m.requestExecutions[msg.ItemID] = &RequestExecution{Status: statusCancelled, Duration: msg.Response.Duration}
	m.statusMessage = fmt.Sprintf("Cancelled: %s", msg.ItemName)
	return m
}
