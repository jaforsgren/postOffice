package textdiff

// maxEditDistance bounds the Myers search. The backtracking trace grows with the
// square of the edit distance, so very different large inputs would otherwise
// allocate unbounded memory.
const maxEditDistance = 1000

// Lines returns a line diff that turns before into after, using the Myers
// algorithm on the section between the common prefix and suffix.
//
// When that section needs more than maxEditDistance edits, the result is still a
// correct diff but not a minimal one: the whole section is reported as deleted
// and then inserted.
func Lines(before, after []string) []Line {
	prefixLen := commonPrefixLen(before, after)
	suffixLen := commonSuffixLen(before[prefixLen:], after[prefixLen:])

	beforeMiddle := before[prefixLen : len(before)-suffixLen]
	afterMiddle := after[prefixLen : len(after)-suffixLen]

	middle, found := myers(beforeMiddle, afterMiddle)
	if !found {
		middle = replaceAll(beforeMiddle, afterMiddle)
	}

	result := linesWithOp(OpEqual, before[:prefixLen])
	result = append(result, middle...)
	return append(result, linesWithOp(OpEqual, before[len(before)-suffixLen:])...)
}

// HasChanges reports whether the diff contains any inserted or deleted lines.
func HasChanges(diff []Line) bool {
	for _, line := range diff {
		if line.Op != OpEqual {
			return true
		}
	}
	return false
}

func commonPrefixLen(a, b []string) int {
	n := 0
	for n < len(a) && n < len(b) && a[n] == b[n] {
		n++
	}
	return n
}

func commonSuffixLen(a, b []string) int {
	n := 0
	for n < len(a) && n < len(b) && a[len(a)-1-n] == b[len(b)-1-n] {
		n++
	}
	return n
}

func linesWithOp(op Op, texts []string) []Line {
	lines := make([]Line, len(texts))
	for i, text := range texts {
		lines[i] = Line{Op: op, Text: text}
	}
	return lines
}

func replaceAll(before, after []string) []Line {
	return append(linesWithOp(OpDelete, before), linesWithOp(OpInsert, after)...)
}

// myers finds a shortest edit script. trace[d] holds the furthest x reached on
// each diagonal k in [-d, d] at the start of round d, stored at index k+d.
func myers(before, after []string) ([]Line, bool) {
	n, m := len(before), len(after)
	maxD := min(n+m, maxEditDistance)
	offset := maxD + 1
	furthest := make([]int, 2*offset+1)
	xAt := func(k int) int { return furthest[offset+k] }

	var trace [][]int
	for d := 0; d <= maxD; d++ {
		trace = append(trace, append([]int(nil), furthest[offset-d:offset+d+1]...))

		for k := -d; k <= d; k += 2 {
			x := xAt(k-1) + 1
			if movesDown(k, d, xAt) {
				x = xAt(k + 1)
			}
			y := x - k
			for x < n && y < m && before[x] == after[y] {
				x++
				y++
			}
			furthest[offset+k] = x

			if x >= n && y >= m {
				return backtrack(trace, before, after), true
			}
		}
	}
	return nil, false
}

// movesDown reports whether diagonal k at round d is reached by an insertion
// (coming from diagonal k+1) rather than a deletion (coming from diagonal k-1).
func movesDown(k, d int, xAt func(int) int) bool {
	return k == -d || (k != d && xAt(k-1) < xAt(k+1))
}

func backtrack(trace [][]int, before, after []string) []Line {
	x, y := len(before), len(after)
	var reversed []Line

	for d := len(trace) - 1; d > 0; d-- {
		window := trace[d]
		xAt := func(k int) int { return window[k+d] }

		k := x - y
		prevK := k - 1
		if movesDown(k, d, xAt) {
			prevK = k + 1
		}
		prevX := xAt(prevK)
		prevY := prevX - prevK

		for x > prevX && y > prevY {
			reversed = append(reversed, Line{Op: OpEqual, Text: before[x-1]})
			x--
			y--
		}

		if x == prevX {
			reversed = append(reversed, Line{Op: OpInsert, Text: after[y-1]})
		} else {
			reversed = append(reversed, Line{Op: OpDelete, Text: before[x-1]})
		}
		x, y = prevX, prevY
	}

	for x > 0 {
		reversed = append(reversed, Line{Op: OpEqual, Text: before[x-1]})
		x--
	}

	result := make([]Line, len(reversed))
	for i, line := range reversed {
		result[len(reversed)-1-i] = line
	}
	return result
}
