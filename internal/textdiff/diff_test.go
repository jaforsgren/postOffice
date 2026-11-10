package textdiff

import (
	"fmt"
	"reflect"
	"testing"
)

func reconstruct(diff []Line) (before, after []string) {
	before, after = []string{}, []string{}
	for _, line := range diff {
		if line.Op != OpInsert {
			before = append(before, line.Text)
		}
		if line.Op != OpDelete {
			after = append(after, line.Text)
		}
	}
	return before, after
}

func countOps(diff []Line) map[Op]int {
	counts := make(map[Op]int)
	for _, line := range diff {
		counts[line.Op]++
	}
	return counts
}

func numbered(prefix string, count int) []string {
	lines := make([]string, count)
	for i := range lines {
		lines[i] = fmt.Sprintf("%s%d", prefix, i)
	}
	return lines
}

func TestLines_ReconstructsBothSides(t *testing.T) {
	tests := []struct {
		name   string
		before []string
		after  []string
	}{
		{"both empty", []string{}, []string{}},
		{"identical", []string{"a", "b", "c"}, []string{"a", "b", "c"}},
		{"only inserts", []string{}, []string{"a", "b"}},
		{"only deletes", []string{"a", "b"}, []string{}},
		{"middle change", []string{"a", "b", "c"}, []string{"a", "x", "c"}},
		{"interleaved", []string{"a", "b", "c", "a", "b", "b", "a"}, []string{"c", "b", "a", "b", "a", "c"}},
		{"append and prepend", []string{"m"}, []string{"s", "m", "e"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			diff := Lines(tt.before, tt.after)
			gotBefore, gotAfter := reconstruct(diff)
			if !reflect.DeepEqual(gotBefore, tt.before) {
				t.Errorf("before reconstruction = %v, want %v", gotBefore, tt.before)
			}
			if !reflect.DeepEqual(gotAfter, tt.after) {
				t.Errorf("after reconstruction = %v, want %v", gotAfter, tt.after)
			}
		})
	}
}

func TestLines_MiddleChangeIsMinimal(t *testing.T) {
	diff := Lines([]string{"a", "b", "c"}, []string{"a", "x", "c"})

	want := []Line{
		{Op: OpEqual, Text: "a"},
		{Op: OpDelete, Text: "b"},
		{Op: OpInsert, Text: "x"},
		{Op: OpEqual, Text: "c"},
	}
	if !reflect.DeepEqual(diff, want) {
		t.Errorf("Lines() = %v, want %v", diff, want)
	}
}

func TestLines_FindsShortestEditScript(t *testing.T) {
	// Classic Myers example: the shortest edit script has 5 edits.
	before := []string{"A", "B", "C", "A", "B", "B", "A"}
	after := []string{"C", "B", "A", "B", "A", "C"}

	counts := countOps(Lines(before, after))
	if edits := counts[OpInsert] + counts[OpDelete]; edits != 5 {
		t.Errorf("edit count = %d, want 5", edits)
	}
}

func TestLines_FallsBackBeyondMaxEditDistance(t *testing.T) {
	before := numbered("before-", maxEditDistance)
	after := numbered("after-", maxEditDistance)

	diff := Lines(before, after)

	gotBefore, gotAfter := reconstruct(diff)
	if !reflect.DeepEqual(gotBefore, before) || !reflect.DeepEqual(gotAfter, after) {
		t.Fatal("fallback diff does not reconstruct the inputs")
	}
	counts := countOps(diff)
	if counts[OpDelete] != maxEditDistance || counts[OpInsert] != maxEditDistance {
		t.Errorf("fallback counts = %v, want %d deletes and inserts", counts, maxEditDistance)
	}
}

func TestLines_KeepsCommonPrefixAndSuffixInFallback(t *testing.T) {
	before := append(append([]string{"head"}, numbered("before-", maxEditDistance)...), "tail")
	after := append(append([]string{"head"}, numbered("after-", maxEditDistance)...), "tail")

	diff := Lines(before, after)

	if diff[0] != (Line{Op: OpEqual, Text: "head"}) {
		t.Errorf("first line = %v, want equal head", diff[0])
	}
	if last := diff[len(diff)-1]; last != (Line{Op: OpEqual, Text: "tail"}) {
		t.Errorf("last line = %v, want equal tail", last)
	}
}

func TestHasChanges(t *testing.T) {
	if HasChanges(Lines([]string{"a"}, []string{"a"})) {
		t.Error("HasChanges() = true for identical input")
	}
	if !HasChanges(Lines([]string{"a"}, []string{"b"})) {
		t.Error("HasChanges() = false for differing input")
	}
}
