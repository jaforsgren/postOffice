package tui

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
)

const (
	confirmKey = "y"
	cancelKey  = "n"
)

// confirmedAction runs once the user answers a confirmation prompt with confirmKey.
type confirmedAction func(Model) (Model, tea.Cmd)

// pendingConfirmation is an action awaiting a y/n answer.
type pendingConfirmation struct {
	prompt          string
	cancelledStatus string
	action          confirmedAction
}

// confirmationPrompt appends the answer keys to question.
func confirmationPrompt(question string) string {
	return fmt.Sprintf("%s (%s/%s)", question, confirmKey, cancelKey)
}

// askConfirmation shows prompt and holds action until the next key press answers it.
func (m Model) askConfirmation(prompt, cancelledStatus string, action confirmedAction) Model {
	m.pendingConfirmation = &pendingConfirmation{
		prompt:          prompt,
		cancelledStatus: cancelledStatus,
		action:          action,
	}
	m.statusMessage = prompt
	return m
}

// confirmDeletion asks before running a destructive action on the thing named by subject.
func (m Model) confirmDeletion(subject string, action confirmedAction) Model {
	return m.askConfirmation(
		confirmationPrompt(fmt.Sprintf("Delete %s?", subject)),
		fmt.Sprintf("Kept %s", subject),
		action,
	)
}

// handleConfirmationKeys answers the pending prompt: confirmKey runs the action, any other
// key cancels, so the prompt can never trap the user.
func (m Model) handleConfirmationKeys(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	pending := *m.pendingConfirmation
	m.pendingConfirmation = nil
	if msg.String() != confirmKey {
		m.statusMessage = pending.cancelledStatus
		return m, nil
	}
	return pending.action(m)
}
