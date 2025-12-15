package tui

const (
	topBarHeight = 7
	statusHeight = 2
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
	totalOverhead := topBarHeight + statusHeight
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
