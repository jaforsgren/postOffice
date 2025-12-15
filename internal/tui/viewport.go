package tui

func sliceContent(lines []string, scrollOffset int, visibleLines int) []string {
	if visibleLines < 1 {
		visibleLines = 1
	}

	startIdx := scrollOffset
	if startIdx > len(lines) {
		startIdx = len(lines)
	}

	endIdx := startIdx + visibleLines
	if endIdx > len(lines) {
		endIdx = len(lines)
	}

	return lines[startIdx:endIdx]
}

func calculateVisibleWindow(cursor int, totalItems int, visibleLines int) (startIdx int, endIdx int) {
	startIdx = 0
	endIdx = totalItems

	if totalItems > visibleLines {
		if cursor > visibleLines/2 {
			startIdx = cursor - visibleLines/2
			if startIdx+visibleLines > totalItems {
				startIdx = totalItems - visibleLines
			}
		}
		endIdx = startIdx + visibleLines
		if endIdx > totalItems {
			endIdx = totalItems
		}
	}

	return startIdx, endIdx
}
