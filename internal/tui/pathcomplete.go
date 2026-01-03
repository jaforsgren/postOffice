package tui

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
)

func parseCommandForPath(input string) (command string, partialPath string, hasSpace bool) {
	parts := strings.Fields(input)
	if len(parts) == 0 {
		return "", "", false
	}

	command = parts[0]

	spaceIdx := strings.Index(input, " ")
	if spaceIdx == -1 {
		return command, "", false
	}

	pathStart := spaceIdx + 1
	for pathStart < len(input) && input[pathStart] == ' ' {
		pathStart++
	}

	if pathStart >= len(input) {
		return command, "", true
	}

	partialPath = input[pathStart:]
	return command, partialPath, true
}

func expandPath(path string) string {
	if strings.HasPrefix(path, "~/") {
		homeDir, err := os.UserHomeDir()
		if err != nil {
			return path
		}
		return filepath.Join(homeDir, path[2:])
	}
	return path
}

func getPathCompletions(partialPath string) []string {
	if partialPath == "" {
		return []string{}
	}

	expandedPath := expandPath(partialPath)

	dir := filepath.Dir(expandedPath)
	base := filepath.Base(expandedPath)

	if partialPath == "~/" || partialPath == "~" {
		dir = expandPath("~/")
		base = ""
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		return []string{}
	}

	var matches []string
	for _, entry := range entries {
		name := entry.Name()

		if strings.HasPrefix(name, ".") {
			continue
		}

		if base != "" && !strings.HasPrefix(strings.ToLower(name), strings.ToLower(base)) {
			continue
		}

		fullPath := filepath.Join(dir, name)

		if strings.HasPrefix(partialPath, "~/") {
			homeDir, err := os.UserHomeDir()
			if err == nil && strings.HasPrefix(fullPath, homeDir) {
				fullPath = "~/" + strings.TrimPrefix(fullPath, homeDir+"/")
			}
		}

		if entry.IsDir() {
			fullPath += "/"
		} else if !strings.HasSuffix(strings.ToLower(name), ".json") {
			continue
		}

		matches = append(matches, fullPath)
	}

	sort.Strings(matches)
	return matches
}

func getPathSuggestion(input string) string {
	command, partialPath, hasSpace := parseCommandForPath(input)
	if !hasSpace {
		return ""
	}

	if command != "load" && command != "l" && command != "loadenv" && command != "le" {
		return ""
	}

	completions := getPathCompletions(partialPath)
	if len(completions) == 0 {
		return ""
	}

	if len(completions) == 1 {
		return command + " " + completions[0]
	}

	commonPrefix := findCommonPrefix(completions)
	if commonPrefix != "" && commonPrefix != partialPath {
		return command + " " + commonPrefix
	}

	return ""
}

func findCommonPrefix(strings []string) string {
	if len(strings) == 0 {
		return ""
	}

	if len(strings) == 1 {
		return strings[0]
	}

	prefix := strings[0]
	for i := 1; i < len(strings); i++ {
		prefix = commonPrefixTwo(prefix, strings[i])
		if prefix == "" {
			break
		}
	}
	return prefix
}

func commonPrefixTwo(a, b string) string {
	minLen := len(a)
	if len(b) < minLen {
		minLen = len(b)
	}

	for i := 0; i < minLen; i++ {
		if a[i] != b[i] {
			return a[:i]
		}
	}
	return a[:minLen]
}
