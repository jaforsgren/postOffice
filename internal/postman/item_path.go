package postman

import "strings"

const (
	// ItemPathSeparator separates folder and request names in an item path
	// such as "Users/Admin/Get User".
	ItemPathSeparator = "/"
	// itemPathEscape escapes a separator or itself inside a name, so names like
	// "unknown policy → 403/404" stay addressable: "unknown policy → 403\/404".
	itemPathEscape = `\`
)

var itemNameEscaper = strings.NewReplacer(itemPathEscape, itemPathEscape+itemPathEscape, ItemPathSeparator, itemPathEscape+ItemPathSeparator)

// EscapeItemName escapes the separator and escape characters in one name.
func EscapeItemName(name string) string {
	return itemNameEscaper.Replace(name)
}

// JoinItemPath builds an item path from raw folder and request names,
// escaping each name so SplitItemPath gives them back unchanged.
func JoinItemPath(names ...string) string {
	escaped := make([]string, len(names))
	for i, name := range names {
		escaped[i] = EscapeItemName(name)
	}
	return strings.Join(escaped, ItemPathSeparator)
}

// SplitItemPath splits an item path into raw names: "\/" is a literal "/" and
// "\\" a literal "\". Any other backslash is kept as written, so paths
// written before escaping existed (a lone "\" in a name) still resolve.
func SplitItemPath(path string) []string {
	var names []string
	var current strings.Builder
	for i := 0; i < len(path); i++ {
		char := path[i : i+1]
		switch {
		case char == itemPathEscape && i+1 < len(path) && (path[i+1:i+2] == ItemPathSeparator || path[i+1:i+2] == itemPathEscape):
			current.WriteString(path[i+1 : i+2])
			i++
		case char == ItemPathSeparator:
			names = append(names, current.String())
			current.Reset()
		default:
			current.WriteString(char)
		}
	}
	return append(names, current.String())
}
