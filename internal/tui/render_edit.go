package tui

import (
	"fmt"
	"strings"

	"postOffice/internal/postman"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

func (m Model) renderEditPopup(availableHeight int) string {
	lines := m.buildEditLines()
	content := strings.Join(lines, "\n")

	return lipgloss.NewStyle().
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("11")).
		Height(availableHeight).
		Width(m.width-4).
		Padding(1, 2).
		Render(content)
}

func (m Model) buildEditLines() []string {
	var lines []string

	title := m.buildEditTitle()
	lines = append(lines, lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("11")).Render(title))
	lines = append(lines, "")

	if m.requestTypeSelectionMode {
		lines = append(lines, m.buildRequestTypeSelectionList()...)
		shortcuts := "<Enter> Select  <j/k> Navigate  <Esc> Cancel"
		lines = append(lines, "")
		lines = append(lines, lipgloss.NewStyle().Foreground(lipgloss.Color("8")).Render(shortcuts))
		return lines
	}

	if m.editRequest != nil {
		lines = append(lines, m.unresolvedVariablesLines(m.editRequest, m.editItemPath)...)
	}
	if m.editType == EditTypeRequest && m.editRequest != nil {
		lines = append(lines, m.buildEditFields()...)
	} else if m.editType == EditTypeGRPCRequest && m.editRequest != nil {
		lines = append(lines, m.buildGRPCEditFields()...)
	}

	lines = append(lines, "")
	shortcuts := m.buildEditShortcuts()
	lines = append(lines, lipgloss.NewStyle().Foreground(lipgloss.Color("8")).Render(shortcuts))

	return lines
}

func (m Model) buildEditShortcuts() string {
	base := "<Enter> Edit field  <j/k> Navigate  <Tab> {{var}} autocomplete  <V> Variables  <Ctrl+G> Open in vim  <Esc> Cancel  <:w> Save  <:wq> Save & Exit"
	if m.editType == EditTypeGRPCRequest {
		return base + "  <Ctrl+F> Reflect"
	}
	return base
}

func (m Model) buildEditTitle() string {
	if m.requestTypeSelectionMode {
		return "Add New Request — Select Type"
	}
	title := "Edit "
	if m.editIsNewItem {
		title = "New "
	}
	switch m.editType {
	case EditTypeRequest:
		if m.editRequest != nil {
			title += "Request: " + m.editRequest.Method
		}
	case EditTypeGRPCRequest:
		if m.grpcEditMethod != "" {
			title += "gRPC Request: " + m.grpcEditMethod
		} else {
			title += "gRPC Request"
		}
	case EditTypeEnvVariable:
		title += "Environment Variable"
	case EditTypeCollectionVariable:
		title += "Collection Variable"
	}
	return title
}

func (m Model) buildGRPCEditFields() []string {
	var lines []string

	metadataText := headersToText(m.editRequest.Header)
	bodyText := ""
	if m.editRequest.Body != nil {
		bodyText = m.editRequest.Body.Raw
	}

	tlsValue := "Disabled (insecure)"
	if m.grpcEditTLS {
		tlsValue = "Enabled"
	}

	fields := []struct {
		label string
		value string
	}{
		{"Name", m.editItemName},
		{"Endpoint", m.grpcEditEndpoint},
		{"Service/Method", m.grpcEditMethod},
		{"Metadata", metadataText},
		{"Message", bodyText},
		{"TLS", tlsValue},
		{"Inject defaults", onOffLabel(m.editRequest.InjectDefaults)},
	}

	dimStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
	tlsEnabledStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("10"))
	tlsDisabledStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("9"))

	for i, field := range fields {
		prefix := "  "
		labelStyle := lipgloss.NewStyle()
		valueStyle := lipgloss.NewStyle()

		if i == m.editFieldCursor {
			prefix = "> "
			labelStyle = labelStyle.Bold(true).Foreground(lipgloss.Color("10"))
			valueStyle = valueStyle.Foreground(lipgloss.Color("12"))
		}

		lines = append(lines, prefix+labelStyle.Render(field.label+":"))

		// Toggle fields: show coloured status + toggle hint, never enter text-edit mode.
		if i == grpcFieldTLS || i == grpcFieldInjectDefaults {
			style := tlsDisabledStyle
			if (i == grpcFieldTLS && m.grpcEditTLS) || (i == grpcFieldInjectDefaults && m.editRequest.InjectDefaults) {
				style = tlsEnabledStyle
			}
			lines = append(lines, "    "+style.Render(field.value))
			if i == m.editFieldCursor {
				lines = append(lines, "    "+dimStyle.Render("[Enter to toggle]"))
			}
			lines = append(lines, "")
			continue
		}

		displayValue := field.value
		if i == m.editFieldCursor && m.editFieldMode {
			if i >= 3 {
				displayValue = m.editFieldTextArea.View()
			} else {
				displayValue = m.editFieldInput.View()
			}
		}

		if displayValue == "" && !m.editFieldMode {
			displayValue = "(empty)"
		}

		valueLines := strings.Split(displayValue, "\n")
		for _, vLine := range valueLines {
			lines = append(lines, "    "+valueStyle.Render(vLine))
		}

		if i == m.editFieldCursor && m.editFieldMode && m.fieldCompletion.active() {
			lines = append(lines, m.buildFieldCompletionDisplay()...)
		}

		// Hint for the Service/Method field
		if i == 2 && i == m.editFieldCursor && !m.editFieldMode {
			lines = append(lines, "    "+dimStyle.Render("[Ctrl+F to browse via reflection]"))
		}

		lines = append(lines, "")
	}

	return lines
}

type editFieldDef struct {
	label    string
	value    string
	isParam  bool
	paramKey string
	isMulti  bool
}

func (m Model) buildEditFields() []string {
	var lines []string

	fields := m.buildRequestFieldDefs()
	dimStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("8"))

	for i, f := range fields {
		isSelected := i == m.editFieldCursor
		prefix := "  "
		labelStyle := lipgloss.NewStyle()
		valueStyle := lipgloss.NewStyle()

		if isSelected {
			prefix = "> "
			labelStyle = labelStyle.Bold(true).Foreground(lipgloss.Color("10"))
			valueStyle = valueStyle.Foreground(lipgloss.Color("12"))
		}

		if f.isParam {
			if isSelected {
				labelStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("13"))
			} else {
				labelStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("13"))
			}
		}

		lines = append(lines, prefix+labelStyle.Render(f.label+":"))

		displayValue := f.value
		if isSelected && m.editFieldMode {
			if f.isMulti {
				displayValue = m.editFieldTextArea.View()
			} else {
				displayValue = m.editFieldInput.View()
			}
		}

		if displayValue == "" && !m.editFieldMode {
			if f.isParam {
				displayValue = dimStyle.Render("(not set)")
			} else {
				displayValue = dimStyle.Render("(empty)")
			}
		}

		valueLines := strings.Split(displayValue, "\n")
		for _, vLine := range valueLines {
			lines = append(lines, "    "+valueStyle.Render(vLine))
		}

		// Variable autocomplete dropdown
		if isSelected && m.editFieldMode && m.fieldCompletion.active() {
			lines = append(lines, m.buildFieldCompletionDisplay()...)
		}

		// Resolved URL preview below URL field
		if i == 2 && !m.editFieldMode && f.value != "" {
			vars := m.variablesAt(m.breadcrumb)
			resolved := postman.ResolveVariables(f.value, vars)
			for name, val := range m.pathParams {
				if val != "" {
					resolvedVal := postman.ResolveVariables(val, vars)
					resolved = strings.ReplaceAll(resolved, ":"+name, resolvedVal)
				}
			}
			resolved = postman.ResolveVariables(resolved, vars)
			if resolved != f.value {
				lines = append(lines, "    "+dimStyle.Italic(true).Render("→ "+resolved))
			}
		}

		// Resolved body preview below Body field
		if f.label == "Body" && !m.editFieldMode && f.value != "" {
			vars := m.variablesAt(m.breadcrumb)
			resolved := postman.ResolveVariables(f.value, vars)
			if resolved != f.value {
				lines = append(lines, "    "+dimStyle.Render("── resolved ──"))
				for _, rl := range strings.Split(resolved, "\n") {
					lines = append(lines, "    "+dimStyle.Italic(true).Render(rl))
				}
			}
		}

		lines = append(lines, "")
	}

	return lines
}

func (m Model) buildRequestFieldDefs() []editFieldDef {
	fields := []editFieldDef{
		{label: "Name", value: m.editItemName},
		{label: "Method", value: m.editRequest.Method},
		{label: "URL", value: m.editRequest.URL.Raw},
	}

	for _, name := range postman.ExtractPathParams(m.editRequest.URL) {
		val := ""
		if m.pathParams != nil {
			val = m.pathParams[name]
		}
		fields = append(fields, editFieldDef{
			label:    ":" + name,
			value:    val,
			isParam:  true,
			paramKey: name,
		})
	}

	headersText := headersToText(m.editRequest.Header)
	bodyText := ""
	if m.editRequest.Body != nil {
		bodyText = m.editRequest.Body.Raw
	}

	fields = append(fields,
		editFieldDef{label: "Headers", value: headersText, isMulti: true},
		editFieldDef{label: "Body", value: bodyText, isMulti: true},
	)

	return fields
}

const (
	maxCompletionRows         = 5
	maxCompletionLabelWidth   = 60
	completionVariablesHeader = "Variables (Tab to insert, Tab again for the next):"
	completionValuesHeader    = "Values seen in requests and responses (Tab to insert, Tab again for the next):"
)

// buildFieldCompletionDisplay renders the autocomplete dropdown, scrolled so the
// selected suggestion stays visible while Tab cycles through them.
func (m Model) buildFieldCompletionDisplay() []string {
	completion := m.fieldCompletion
	dim := lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
	selectedStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("14")).Bold(true)

	header := completionVariablesHeader
	if completion.kind == completionFieldValues {
		header = completionValuesHeader
	}
	lines := []string{"    " + dim.Render(header)}

	startIdx, endIdx := calculateVisibleWindow(completion.cursor, len(completion.items), maxCompletionRows)
	for i := startIdx; i < endIdx; i++ {
		item := completion.items[i]
		label := ansi.Truncate(item.label, maxCompletionLabelWidth, "…")
		detail := dim.Render("  " + item.detail)
		if i == completion.cursor {
			lines = append(lines, "    > "+selectedStyle.Render(label)+detail)
		} else {
			lines = append(lines, "      "+dim.Render(label)+detail)
		}
	}

	if hidden := len(completion.items) - (endIdx - startIdx); hidden > 0 {
		lines = append(lines, "      "+dim.Render(fmt.Sprintf("... and %d more", hidden)))
	}
	return lines
}

func (m Model) buildRequestTypeSelectionList() []string {
	var lines []string

	for i, option := range m.items {
		prefix := "  "
		style := lipgloss.NewStyle()

		if i == m.cursor {
			prefix = "> "
			style = style.Bold(true).Foreground(lipgloss.Color("10"))
		}

		lines = append(lines, prefix+style.Render(option))
	}

	return lines
}
