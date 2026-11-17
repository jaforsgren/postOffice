package tui

import (
	"context"
	"fmt"
	"postOffice/internal/engine"
	"postOffice/internal/postman"
	"postOffice/internal/script"
	"postOffice/internal/servicebus"
	"slices"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

// resentItemSuffix tells a resent message apart from its PEEK request in
// the history.
const resentItemSuffix = " (resent)"

// listenedSendItem is a request that sends listened again, named after it.
func listenedSendItem(listened listenedMessage) (postman.Item, error) {
	request, err := servicebus.SendRequest(listened.source.request, listened.message)
	if err != nil {
		return postman.Item{}, err
	}
	name := fmt.Sprintf("%s #%d", listened.source.name, listened.message.SequenceNumber)
	if label := messageLabel(listened); label != "" {
		name = fmt.Sprintf("%s #%d", label, listened.message.SequenceNumber)
	}
	return postman.Item{Name: name, Request: request}, nil
}

// handleResendListenedMessageKey sends the selected message again to the
// queue or topic it was peeked from, running the pre-request scripts of the
// PEEK request's collection and folder. Its listener shows it arriving.
func handleResendListenedMessageKey(m Model) (Model, tea.Cmd) {
	selected, ok := m.selectedListenedMessage()
	if !ok {
		m.statusMessage = errNoListenedMessage.Error()
		return m, nil
	}
	item, err := listenedSendItem(selected)
	if err != nil {
		return m.listenedMessageFailed("resend", err), nil
	}
	collection, exists := m.parser.GetCollection(selected.source.collectionName)
	if !exists {
		return m.listenedMessageFailed("resend", fmt.Errorf("collection %s is no longer loaded", selected.source.collectionName)), nil
	}
	environment := m.environment
	execution, err := m.engine.Prepare(engine.Call{
		Item: &item,
		Scope: script.RequestScope{
			Collection:  collection,
			Environment: environment,
			FolderPath:  slices.Clone(selected.source.folderPath),
			Item:        &item,
		},
	})
	if err != nil {
		return m.listenedMessageFailed("resend", err), nil
	}

	itemID := selected.source.itemID + resentItemSuffix
	m.requestExecutions[itemID] = &RequestExecution{Status: statusSending, Timestamp: time.Now()}
	environmentName := m.environmentName()
	m.statusMessage = fmt.Sprintf("Resending #%d to %s", selected.message.SequenceNumber, item.Request.URL.Raw)
	return m, func() tea.Msg {
		result := execution.Run(context.Background())
		return RequestCompleteMsg{
			ItemID:          itemID,
			Response:        result.Response,
			TestResult:      result.TestResult,
			Collection:      collection,
			Environment:     environment,
			EnvironmentName: environmentName,
			Protocol:        result.Protocol,
			ItemName:        item.Name + resentItemSuffix,
		}
	}
}

// handleSaveListenedMessageKey saves the selected message as a request that
// sends it, next to the PEEK request it was seen by.
func handleSaveListenedMessageKey(m Model) (Model, tea.Cmd) {
	selected, ok := m.selectedListenedMessage()
	if !ok {
		m.statusMessage = errNoListenedMessage.Error()
		return m, nil
	}
	item, err := listenedSendItem(selected)
	if err != nil {
		return m.listenedMessageFailed("save", err), nil
	}
	source := selected.source
	collection, exists := m.parser.GetCollection(source.collectionName)
	if !exists {
		return m.listenedMessageFailed("save", fmt.Errorf("collection %s is no longer loaded", source.collectionName)), nil
	}
	items := traverseToDepthPtr(&collection.Items, source.folderPath)
	if items == nil {
		return m.listenedMessageFailed("save", fmt.Errorf("folder %s is gone", postman.JoinItemPath(source.folderPath...))), nil
	}
	*items = append(*items, item)
	if err := m.saveCollectionUndoable(source.collectionName); err != nil {
		return m.listenedMessageFailed("save", err), nil
	}
	m = m.reloadMergedCollection().refreshCurrentView()
	m.statusMessage = fmt.Sprintf("Saved request %q in %s", item.Name, postman.JoinItemPath(append([]string{source.collectionName}, source.folderPath...)...))
	return m, nil
}

func (m Model) listenedMessageFailed(action string, err error) Model {
	m.statusMessage = fmt.Sprintf("Cannot %s the message: %v", action, err)
	m.statusIsError = true
	return m
}
