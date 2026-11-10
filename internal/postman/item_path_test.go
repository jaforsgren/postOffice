package postman

import (
	"slices"
	"testing"
)

func TestItemPath_RoundTrip(t *testing.T) {
	cases := [][]string{
		{"Users", "Get User"},
		{"NEGATIVE", "unknown policy → 403/404"},
		{"a/b", "c\\d", "trailing\\"},
		{"//", "\\/"},
		{"single"},
		{""},
	}
	for _, names := range cases {
		path := JoinItemPath(names...)
		if got := SplitItemPath(path); !slices.Equal(got, names) {
			t.Errorf("SplitItemPath(JoinItemPath(%q)) = %q (path %q)", names, got, path)
		}
	}
}

func TestJoinItemPath_Escapes(t *testing.T) {
	cases := map[string][]string{
		`Users/Get User`:                     {"Users", "Get User"},
		`NEGATIVE/unknown policy → 403\/404`: {"NEGATIVE", "unknown policy → 403/404"},
		`back\\slash`:                        {`back\slash`},
	}
	for want, names := range cases {
		if got := JoinItemPath(names...); got != want {
			t.Errorf("JoinItemPath(%q) = %q, want %q", names, got, want)
		}
	}
}

func TestSplitItemPath_KeepsUnknownEscapesLiteral(t *testing.T) {
	cases := map[string][]string{
		`C:\temp/file`: {`C:\temp`, "file"},
		`ends with\`:   {`ends with\`},
		`a//b`:         {"a", "", "b"},
	}
	for path, want := range cases {
		if got := SplitItemPath(path); !slices.Equal(got, want) {
			t.Errorf("SplitItemPath(%q) = %q, want %q", path, got, want)
		}
	}
}
