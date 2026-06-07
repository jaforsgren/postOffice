package tui

import (
	"fmt"
	"strings"

	"postOffice/internal/postman"

	"github.com/charmbracelet/lipgloss"
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

	if m.scriptSelectionMode {
		lines = append(lines, m.buildScriptSelectionList()...)
		shortcuts := "<Enter> Select  <j/k> Navigate  <Esc> Cancel"
		lines = append(lines, "")
		lines = append(lines, lipgloss.NewStyle().Foreground(lipgloss.Color("8")).Render(shortcuts))
		return lines
	}

	if m.editType == EditTypeScript {
		lines = append(lines, m.buildScriptEditor()...)
		shortcuts := "<:w> Save  <:wq> Save & Exit  <Esc> Cancel"
		lines = append(lines, "")
		lines = append(lines, lipgloss.NewStyle().Foreground(lipgloss.Color("8")).Render(shortcuts))
		return lines
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
	base := "<Enter> Edit field  <j/k> Navigate  <Tab> {{var}} autocomplete  <Esc> Cancel  <:w> Save  <:wq> Save & Exit"
	if m.editType == EditTypeGRPCRequest {
		return base + "  <Ctrl+R> Reflect"
	}
	return base
}

func (m Model) buildEditTitle() string {
	title := "Edit "
	switch m.editType {
	case EditTypeRequest:
		if m.editRequest != nil {
			title += "Request: " + m.editRequest.Method
		}
	case EditTypeGRPCRequest:
		if m.grpcEditMethod != "" {
			title = "Edit gRPC Request: " + m.grpcEditMethod
		} else {
			title = "Edit gRPC Request"
		}
	case EditTypeEnvVariable:
		title += "Environment Variable"
	case EditTypeCollectionVariable:
		title += "Collection Variable"
	case EditTypeScript:
		if m.scriptSelectionMode {
			title = "Select Script Type - " + m.editScriptItemName
		} else {
			scriptTypeName := "Pre-request"
			if m.editScriptType == ScriptTypeTest {
				scriptTypeName = "Test"
			}
			title = scriptTypeName + " Script - " + m.editScriptItemName
		}
	case EditTypeWorkflowStepScript:
		if m.scriptSelectionMode {
			title = "Select Script Type - Step " + m.editScriptItemName
		} else {
			scriptTypeName := "Pre-step"
			if m.editScriptType == ScriptTypeStepPost {
				scriptTypeName = "Post-step"
			}
			title = scriptTypeName + " Script - Step " + m.editScriptItemName
		}
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

		// TLS field: show coloured status + toggle hint, never enter text-edit mode.
		if i == 5 {
			style := tlsDisabledStyle
			if m.grpcEditTLS {
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

		if i == m.editFieldCursor && m.editFieldMode && m.varSuggestionActive {
			lines = append(lines, m.buildVarSuggestionsDisplay()...)
		}

		// Hint for the Service/Method field
		if i == 2 && i == m.editFieldCursor && !m.editFieldMode {
			lines = append(lines, "    "+dimStyle.Render("[Ctrl+R to browse via reflection]"))
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
		if isSelected && m.editFieldMode && m.varSuggestionActive {
			lines = append(lines, m.buildVarSuggestionsDisplay()...)
		}

		// Resolved URL preview below URL field
		if i == 2 && !m.editFieldMode && f.value != "" {
			vars := postman.GetAllVariables(m.collection, m.breadcrumb, m.environment)
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

func (m Model) buildVarSuggestionsDisplay() []string {
	if len(m.varSuggestions) == 0 {
		return nil
	}

	var lines []string
	headerStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
	lines = append(lines, "    "+headerStyle.Render("Variables (Tab to insert):"))

	maxShow := 5
	if len(m.varSuggestions) < maxShow {
		maxShow = len(m.varSuggestions)
	}

	for i := 0; i < maxShow; i++ {
		v := m.varSuggestions[i]
		varStr := "{{" + v.Key + "}}"

		resolvedDisplay := ""
		if v.Value != "" {
			val := v.Value
			if len(val) > 35 {
				val = val[:32] + "..."
			}
			resolvedDisplay = " = " + val
		}

		if i == m.varSuggestionCursor {
			style := lipgloss.NewStyle().Foreground(lipgloss.Color("14")).Bold(true)
			lines = append(lines, "    > "+style.Render(varStr+resolvedDisplay))
		} else {
			style := lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
			lines = append(lines, "      "+style.Render(varStr+resolvedDisplay))
		}
	}

	if len(m.varSuggestions) > maxShow {
		dim := lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
		lines = append(lines, "      "+dim.Render(fmt.Sprintf("... and %d more", len(m.varSuggestions)-maxShow)))
	}

	return lines
}

func (m Model) buildScriptSelectionList() []string {
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

func (m Model) buildScriptEditor() []string {
	var lines []string

	lineCount := strings.Count(m.editFieldTextArea.Value(), "\n") + 1
	lines = append(lines, lipgloss.NewStyle().Foreground(lipgloss.Color("8")).Render(fmt.Sprintf("Lines: %d", lineCount)))
	lines = append(lines, "")
	lines = append(lines, m.editFieldTextArea.View())

	return lines
}
