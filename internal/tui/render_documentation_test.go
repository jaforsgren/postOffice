package tui

import (
	"slices"
	"strings"
	"testing"

	"postOffice/internal/postman"
)

const (
	testTerminalWidth  = 160
	testTerminalHeight = 50
)

func TestFolderInfo_leadsWithDocumentationWrappedToWidth(t *testing.T) {
	const width = 24
	m := Model{currentInfoItem: &postman.Item{
		Name:        "Payments",
		Description: "Events for the payments topic, sent to the emulator\nSecond line",
		Items:       []postman.Item{{Name: "send", Request: &postman.Request{Method: "POST"}}},
	}}

	lines := m.buildItemDetailLines(width)

	docs := slices.Index(lines, requestStyle.Render(documentationHeading))
	typeHeading := slices.Index(lines, requestStyle.Render("Type:"))
	if docs < 0 || typeHeading < docs {
		t.Fatalf("expected documentation before the folder type, got:\n%s", strings.Join(lines, "\n"))
	}
	for _, line := range lines[docs+1 : typeHeading] {
		if len(line) > width {
			t.Errorf("line %q is wider than %d", line, width)
		}
	}
	joined := strings.Join(lines, "\n")
	for _, want := range []string{infoIndent + "Events for the", infoIndent + "Second line"} {
		if !strings.Contains(joined, want) {
			t.Errorf("expected %q in:\n%s", want, joined)
		}
	}
}

func TestFolderInfo_withoutDocumentationHasNoSection(t *testing.T) {
	m := Model{currentInfoItem: &postman.Item{Name: "Plain", Items: []postman.Item{{Name: "send", Request: &postman.Request{Method: "GET"}}}}}

	if lines := m.buildItemDetailLines(80); slices.Contains(lines, requestStyle.Render(documentationHeading)) {
		t.Errorf("expected no documentation section, got:\n%s", strings.Join(lines, "\n"))
	}
}

func TestSplitInfoLines_CombinedViewCollectionShowsItsDocumentation(t *testing.T) {
	m, _, _ := combinedModel(t)
	b, _ := m.parser.GetCollection(combinedB)
	b.Info.Description = "Collection B docs"
	m.mode = ModeCollections
	m = m.loadCollectionsList()
	m.cursor = 0
	m = cursorOn(t, m.handleSelection(), combinedB)
	m.width, m.height = testTerminalWidth, testTerminalHeight

	info := strings.Join(m.splitInfoLines(), "\n")

	if !strings.Contains(info, documentationHeading) || !strings.Contains(info, "Collection B docs") {
		t.Errorf("expected collection B's documentation, got:\n%s", info)
	}
}
