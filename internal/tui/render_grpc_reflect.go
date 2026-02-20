package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

func (m Model) renderGRPCReflect(availableHeight int) string {
	var lines []string

	titleStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("14"))
	dimStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("8"))

	lines = append(lines, titleStyle.Render("gRPC Reflection — "+m.grpcEditEndpoint))
	lines = append(lines, "")

	if m.grpcReflectPhase == 0 {
		lines = append(lines, m.renderServiceList()...)
		lines = append(lines, "")
		shortcuts := "<Enter> View Methods  <j/k> Navigate  <Esc> Back to Edit"
		lines = append(lines, dimStyle.Render(shortcuts))
	} else {
		if m.grpcSelectedService < len(m.grpcReflectServices) {
			svc := m.grpcReflectServices[m.grpcSelectedService]
			lines = append(lines, dimStyle.Render(fmt.Sprintf("Service: %s", svc.Name)))
			lines = append(lines, "")
			lines = append(lines, m.renderMethodList(svc.Name)...)
		}
		lines = append(lines, "")
		shortcuts := "<Enter> Use Method  <j/k> Navigate  <Esc> Back to Services"
		lines = append(lines, dimStyle.Render(shortcuts))
	}

	content := strings.Join(lines, "\n")
	return mainWindowStyle.
		Height(availableHeight).
		Width(m.width - 4).
		Render(content)
}

func (m Model) renderServiceList() []string {
	var lines []string
	for i, svc := range m.grpcReflectServices {
		prefix := "  "
		style := lipgloss.NewStyle()
		if i == m.grpcSelectedService {
			prefix = "> "
			style = style.Bold(true).Foreground(lipgloss.Color("10"))
		}
		methodCount := len(svc.Methods)
		label := fmt.Sprintf("%s (%d method(s))", svc.Name, methodCount)
		lines = append(lines, prefix+style.Render(label))
	}
	return lines
}

func (m Model) renderMethodList(serviceName string) []string {
	var lines []string
	if m.grpcSelectedService >= len(m.grpcReflectServices) {
		return lines
	}
	svc := m.grpcReflectServices[m.grpcSelectedService]
	if svc.Name != serviceName {
		return lines
	}
	for i, method := range svc.Methods {
		prefix := "  "
		style := lipgloss.NewStyle()
		if i == m.cursor {
			prefix = "> "
			style = style.Bold(true).Foreground(lipgloss.Color("12"))
		}
		lines = append(lines, prefix+style.Render(method.Name))
	}
	return lines
}
