package tui

const (
	statusHeight     = 2
	commandBarHeight = 1
)

type layoutMetrics struct {
	topBarHeight  int
	statusHeight  int
	totalOverhead int
	contentHeight int
	mainHeight    int
	popupHeight   int
}

func (m Model) calculateLayout() layoutMetrics {
	// Content views are bordered, and lipgloss Height excludes the border.
	totalOverhead := topBarHeight + statusHeight + commandBarHeight + paneBorderSize
	contentHeight := m.height - totalOverhead

	return layoutMetrics{
		topBarHeight:  topBarHeight,
		statusHeight:  statusHeight,
		totalOverhead: totalOverhead,
		contentHeight: contentHeight,
	}
}

func (m Model) calculateSplitLayout() layoutMetrics {
	metrics := m.calculateLayout()
	metrics.mainHeight = metrics.contentHeight / 2
	metrics.popupHeight = metrics.contentHeight - metrics.mainHeight

	return metrics
}

const (
	windowWidthInset      = 4
	paneBorderSize        = 2
	paneVerticalPadding   = 1
	paneHorizontalPadding = 2
	splitListWidthPercent = 40
	percentScale          = 100
	minPaneSize           = 1
)

// requestSplitLayout holds lipgloss Width/Height values for the panes of the
// requests split view. Like lipgloss, they include padding but not the border.
type requestSplitLayout struct {
	listWidth      int
	listHeight     int
	rightWidth     int
	infoHeight     int
	responseHeight int
}

// calculateRequestSplitLayout divides the single-pane main window so that the
// bordered panes together occupy exactly the same outer size.
func (m Model) calculateRequestSplitLayout() requestSplitLayout {
	metrics := m.calculateLayout()

	outerWidth := m.width - windowWidthInset + paneBorderSize
	listOuterWidth := outerWidth * splitListWidthPercent / percentScale
	rightOuterWidth := outerWidth - listOuterWidth

	outerHeight := metrics.contentHeight + paneBorderSize
	infoOuterHeight := outerHeight / 2
	responseOuterHeight := outerHeight - infoOuterHeight

	return requestSplitLayout{
		listWidth:      max(listOuterWidth-paneBorderSize, minPaneSize),
		listHeight:     max(metrics.contentHeight, minPaneSize),
		rightWidth:     max(rightOuterWidth-paneBorderSize, minPaneSize),
		infoHeight:     max(infoOuterHeight-paneBorderSize, minPaneSize),
		responseHeight: max(responseOuterHeight-paneBorderSize, minPaneSize),
	}
}

func paneInnerWidth(width int) int {
	return max(width-2*paneHorizontalPadding, minPaneSize)
}

func paneInnerHeight(height int) int {
	return max(height-2*paneVerticalPadding, minPaneSize)
}
