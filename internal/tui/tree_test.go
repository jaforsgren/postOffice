package tui

import (
	"postOffice/internal/postman"
	"strings"
	"testing"
)

// nestedMockCollection has a folder-in-folder structure so tree expand/collapse
// can be exercised down to two levels: Outer/Inner/Leaf Request, plus a
// top-level request as a sibling of Outer.
func nestedMockCollection() *postman.Collection {
	return &postman.Collection{
		Info: postman.Info{Name: "Nested Collection"},
		Items: []postman.Item{
			{
				Name: "Outer",
				Items: []postman.Item{
					{
						Name: "Inner",
						Items: []postman.Item{
							{
								Name:    "Leaf Request",
								Request: &postman.Request{Method: "GET", URL: postman.URL{Raw: "https://example.com/leaf"}},
							},
						},
					},
					{
						Name:    "Outer Request",
						Request: &postman.Request{Method: "GET", URL: postman.URL{Raw: "https://example.com/outer"}},
					},
				},
			},
			{
				Name:    "Top Request",
				Request: &postman.Request{Method: "POST", URL: postman.URL{Raw: "https://example.com/top"}},
			},
		},
	}
}

func nestedTestModel() Model {
	parser := postman.NewParser()
	collection := nestedMockCollection()
	m := NewModel(parser)
	m.collection = collection
	m.mode = ModeRequests
	m = m.loadRequestsList()
	return m
}

func TestToggleExpand_CollapsedByDefault(t *testing.T) {
	m := nestedTestModel()

	if len(m.currentItems) != 2 {
		t.Fatalf("expected 2 top-level items collapsed, got %d", len(m.currentItems))
	}
	if !strings.Contains(m.items[0], "▸") {
		t.Errorf("expected collapsed folder marker ▸, got %q", m.items[0])
	}
}

func TestToggleExpand_ExpandsOneLevel(t *testing.T) {
	m := nestedTestModel()
	m.cursor = 0 // "Outer" folder

	m = m.toggleExpandAtCursor()

	if len(m.currentItems) != 4 {
		t.Fatalf("expected 4 items after expanding Outer (Outer, Inner, Outer Request, Top Request), got %d", len(m.currentItems))
	}
	names := []string{}
	for _, item := range m.currentItems {
		names = append(names, item.Name)
	}
	want := []string{"Outer", "Inner", "Outer Request", "Top Request"}
	for i, w := range want {
		if names[i] != w {
			t.Errorf("position %d: expected %q, got %q (full: %v)", i, w, names[i], names)
		}
	}
	if !strings.Contains(m.items[0], "▾") {
		t.Errorf("expected expanded folder marker ▾ on Outer, got %q", m.items[0])
	}
}

func TestToggleExpand_CollapseRestoresOriginalList(t *testing.T) {
	m := nestedTestModel()
	m.cursor = 0

	m = m.toggleExpandAtCursor() // expand
	m = m.toggleExpandAtCursor() // collapse

	if len(m.currentItems) != 2 {
		t.Fatalf("expected 2 items after collapsing, got %d", len(m.currentItems))
	}
	if strings.Contains(m.items[0], "▾") {
		t.Errorf("expected collapsed marker after re-toggle, got %q", m.items[0])
	}
}

func TestToggleExpand_NestedFolderPaths(t *testing.T) {
	m := nestedTestModel()
	m.cursor = 0
	m = m.toggleExpandAtCursor() // expand Outer -> Outer, Inner, Outer Request, Top Request

	innerIdx := -1
	for i, item := range m.currentItems {
		if item.Name == "Inner" {
			innerIdx = i
		}
	}
	if innerIdx == -1 {
		t.Fatal("expected to find Inner folder in flattened tree")
	}

	m.cursor = innerIdx
	m = m.toggleExpandAtCursor() // expand Inner -> reveals Leaf Request

	leafIdx := -1
	for i, item := range m.currentItems {
		if item.Name == "Leaf Request" {
			leafIdx = i
		}
	}
	if leafIdx == -1 {
		t.Fatalf("expected Leaf Request to be visible after expanding Inner, items: %v", m.currentItems)
	}

	gotPath := m.folderPathForIndex(leafIdx)
	wantPath := []string{"Outer", "Inner"}
	if len(gotPath) != len(wantPath) || gotPath[0] != wantPath[0] || gotPath[1] != wantPath[1] {
		t.Errorf("expected Leaf Request folder path %v, got %v", wantPath, gotPath)
	}

	// Top-level breadcrumb must remain unchanged by expansion — expand is a
	// browsing aid, not navigation.
	if len(m.breadcrumb) != 0 {
		t.Errorf("expected breadcrumb to stay at root while browsing via tree expand, got %v", m.breadcrumb)
	}
}

func TestToggleExpand_OnRequestIsNoOp(t *testing.T) {
	m := nestedTestModel()
	m.cursor = 1 // "Top Request"

	before := len(m.currentItems)
	m = m.toggleExpandAtCursor()

	if len(m.currentItems) != before {
		t.Errorf("expected no structural change when toggling a request, got %d items (was %d)", len(m.currentItems), before)
	}
	if m.statusMessage != "Only folders can be expanded" {
		t.Errorf("unexpected status message: %q", m.statusMessage)
	}
}

func TestNavigateIntoAt_NestedExpandedFolder(t *testing.T) {
	m := nestedTestModel()
	m.cursor = 0
	m = m.toggleExpandAtCursor() // expand Outer

	innerIdx := -1
	for i, item := range m.currentItems {
		if item.Name == "Inner" {
			innerIdx = i
		}
	}
	if innerIdx == -1 {
		t.Fatal("expected Inner to be visible")
	}

	item := m.currentItems[innerIdx]
	m = m.navigateIntoAt(item, m.folderPathForIndex(innerIdx))

	want := []string{"Outer", "Inner"}
	if len(m.breadcrumb) != 2 || m.breadcrumb[0] != want[0] || m.breadcrumb[1] != want[1] {
		t.Errorf("expected breadcrumb %v after navigating into nested expanded folder, got %v", want, m.breadcrumb)
	}
	if len(m.currentItems) != 1 || m.currentItems[0].Name != "Leaf Request" {
		t.Errorf("expected to be browsing Inner's contents (Leaf Request), got %v", m.currentItems)
	}
}

func TestToggleExpandAll_ExpandsEveryFolderRecursively(t *testing.T) {
	m := nestedTestModel()

	m = m.toggleExpandAllAtCurrentLevel()

	names := []string{}
	for _, item := range m.currentItems {
		names = append(names, item.Name)
	}
	want := []string{"Outer", "Inner", "Leaf Request", "Outer Request", "Top Request"}
	if len(names) != len(want) {
		t.Fatalf("expected fully expanded tree %v, got %v", want, names)
	}
	for i, w := range want {
		if names[i] != w {
			t.Errorf("position %d: expected %q, got %q (full: %v)", i, w, names[i], names)
		}
	}
	if m.statusMessage != "Expanded all folders" {
		t.Errorf("unexpected status message: %q", m.statusMessage)
	}
}

func TestToggleExpandAll_CollapsesWhenAllAlreadyExpanded(t *testing.T) {
	m := nestedTestModel()
	m = m.toggleExpandAllAtCurrentLevel() // expand all

	m = m.toggleExpandAllAtCurrentLevel() // should collapse all back

	if len(m.currentItems) != 2 {
		t.Fatalf("expected 2 top-level items after collapsing all, got %d", len(m.currentItems))
	}
	if m.statusMessage != "Collapsed all folders" {
		t.Errorf("unexpected status message: %q", m.statusMessage)
	}
}

func TestToggleExpandAll_PartiallyExpandedExpandsRest(t *testing.T) {
	m := nestedTestModel()
	m.cursor = 0
	m = m.toggleExpandAtCursor() // expand only Outer, Inner stays collapsed

	m = m.toggleExpandAllAtCurrentLevel()

	names := []string{}
	for _, item := range m.currentItems {
		names = append(names, item.Name)
	}
	want := []string{"Outer", "Inner", "Leaf Request", "Outer Request", "Top Request"}
	if len(names) != len(want) {
		t.Fatalf("expected fully expanded tree %v, got %v", want, names)
	}
	if m.statusMessage != "Expanded all folders" {
		t.Errorf("expected 'Expanded all folders' when partially expanded, got %q", m.statusMessage)
	}
}

func TestToggleExpandAll_NoFolders(t *testing.T) {
	m := nestedTestModel()
	m.cursor = 0
	m = m.toggleExpandAtCursor() // expand Outer
	innerIdx := -1
	for i, item := range m.currentItems {
		if item.Name == "Inner" {
			innerIdx = i
		}
	}
	m.cursor = innerIdx
	m = m.toggleExpandAtCursor() // expand Inner too, so everything is already flat

	// Navigate into the deepest folder, which has no sub-folders left to expand.
	m = m.navigateIntoAt(postman.Item{Name: "Inner", Items: []postman.Item{
		{Name: "Leaf Request", Request: &postman.Request{Method: "GET", URL: postman.URL{Raw: "https://example.com/leaf"}}},
	}}, []string{"Outer"})

	m = m.toggleExpandAllAtCurrentLevel()

	if m.statusMessage != "No folders to expand" {
		t.Errorf("expected 'No folders to expand', got %q", m.statusMessage)
	}
}

func TestFormatItemLine_TreeIndentationForExpandedChildren(t *testing.T) {
	m := nestedTestModel()
	m.cursor = 0
	m = m.toggleExpandAtCursor()
	m.width = 200

	innerLine := ""
	for i, item := range m.currentItems {
		if item.Name == "Inner" {
			innerLine = m.formatItemLine(i, itemNameWidth)
		}
	}
	if innerLine == "" {
		t.Fatal("expected to find rendered line for Inner")
	}
	if !strings.Contains(innerLine, "  [DIR]") {
		t.Errorf("expected Inner's rendered line to be indented, got %q", innerLine)
	}
}
