package tui

import (
	"encoding/json"
	"fmt"
	"maps"
	"slices"
	"strings"

	"postOffice/internal/http"
	"postOffice/internal/script"
)

// requestReportFilePattern gives the temp file a .md extension so the editor
// highlights headings and the fenced snippets by language.
const requestReportFilePattern = "postoffice-request-*.md"

const (
	markdownFence    = "```"
	langJSON         = "json"
	langXML          = "xml"
	langHTTP         = "http"
	langJavaScript   = "javascript"
	langShell        = "bash"
	langText         = "text"
	untitledRequest  = "Request"
	prerequestListen = "prerequest"
	testScriptListen = "test"
	testPassedMarker = "- [x] "
	testFailedMarker = "- [ ] "
	blankLine        = ""
	// sectionBreak sets what came back apart from what was sent.
	sectionBreak = "---"
)

// markdownReport is the request report as Markdown: one section per part,
// with headers, bodies and scripts as fenced snippets. Like the response
// view, it puts what came back first (the response and how it tested), then
// what was sent and how the request is defined, the curl to repeat it and
// the scripts last.
func (t yankTarget) markdownReport() (string, error) {
	if t.definition == nil && t.response == nil {
		return "", errNothingToOpen
	}

	var lines []string
	lines = append(lines, "# "+t.reportTitle(), blankLine)
	if t.response != nil {
		lines = append(lines, responseSection(t.response)...)
		if t.model.lastTestResult != nil {
			lines = append(lines, testResultSection(t.model.lastTestResult)...)
		}
	}
	if len(t.stepLogs) > 0 {
		lines = append(lines, "## Logs", blankLine)
		lines = append(lines, fencedBlock(langText, strings.Join(t.stepLogs, "\n"))...)
	}
	if t.response != nil {
		lines = append(lines, sectionBreak, blankLine)
		lines = append(lines, sentRequestSection(t.response)...)
	}
	if t.definition != nil {
		lines = append(lines, t.definitionSection()...)
	}
	curl, kind := t.curl()
	lines = append(lines, "## "+kind, blankLine)
	lines = append(lines, fencedBlock(langShell, curl)...)
	if t.definition != nil {
		lines = append(lines, t.scriptsAndDescription()...)
	}

	return strings.Join(lines, "\n"), nil
}

func (t yankTarget) reportTitle() string {
	if t.definition != nil && t.definition.Name != "" {
		return t.definition.Name
	}
	if t.response != nil {
		return t.response.RequestMethod + " " + t.response.RequestURL
	}
	return untitledRequest
}

// definitionSection is the request as saved. Once it was sent, its headers
// and body are left to the sent request section, which shows them resolved.
func (t yankTarget) definitionSection() []string {
	request := t.definition.Request
	lines := []string{"## Definition", blankLine}
	lines = append(lines, fmt.Sprintf("- **Method:** `%s`  **URL:** `%s`", request.Method, request.URL.Raw))
	if resolved := t.url(); resolved != request.URL.Raw {
		lines = append(lines, fmt.Sprintf("- **Resolved:** `%s`", resolved))
	}
	lines = append(lines, blankLine)
	if t.response != nil {
		return lines
	}

	if len(request.Header) > 0 {
		var headers []string
		for _, header := range request.Header {
			headers = append(headers, header.Key+": "+header.Value)
		}
		lines = append(lines, "### Headers", blankLine)
		lines = append(lines, fencedBlock(langHTTP, strings.Join(headers, "\n"))...)
	}
	if request.Body != nil && request.Body.Raw != "" {
		lines = append(lines, "### Body", blankLine)
		lines = append(lines, bodyBlock(request.Body.Raw)...)
	}
	return lines
}

func (t yankTarget) scriptsAndDescription() []string {
	var lines []string
	for _, event := range t.definition.Events {
		if len(event.Script.Exec) == 0 {
			continue
		}
		lines = append(lines, "### "+scriptHeading(event.Listen), blankLine)
		lines = append(lines, fencedBlock(langJavaScript, strings.Join(event.Script.Exec, "\n"))...)
	}
	if t.definition.Description != "" {
		lines = append(lines, "### Description", blankLine, t.definition.Description, blankLine)
	}
	return lines
}

func scriptHeading(listen string) string {
	switch listen {
	case prerequestListen:
		return "Pre-request script"
	case testScriptListen:
		return "Test script"
	}
	return listen + " script"
}

// sentRequestSection is the request as it went over the wire, in HTTP
// message form: request line, headers, blank line, body.
func sentRequestSection(response *http.Response) []string {
	message := []string{response.RequestMethod + " " + response.RequestURL}
	for _, key := range slices.Sorted(maps.Keys(response.RequestHeaders)) {
		message = append(message, key+": "+response.RequestHeaders[key])
	}

	lines := []string{"## Sent request", blankLine}
	lines = append(lines, fencedBlock(langHTTP, strings.Join(message, "\n"))...)
	if response.RequestBody != "" {
		lines = append(lines, "### Body", blankLine)
		lines = append(lines, bodyBlock(response.RequestBody)...)
	}
	return lines
}

func responseSection(response *http.Response) []string {
	lines := []string{"## Response", blankLine}
	if response.Error != nil {
		lines = append(lines, "**Error:**", blankLine)
		return append(lines, fencedBlock(langText, response.Error.Error())...)
	}

	lines = append(lines, fmt.Sprintf("**Status:** `%s`  **Duration:** %s", response.Status, response.Duration), blankLine)

	if len(response.Headers) > 0 {
		var headers []string
		for _, key := range slices.Sorted(maps.Keys(response.Headers)) {
			for _, value := range response.Headers[key] {
				headers = append(headers, key+": "+value)
			}
		}
		lines = append(lines, "### Headers", blankLine)
		lines = append(lines, fencedBlock(langHTTP, strings.Join(headers, "\n"))...)
	}
	if response.Body != "" {
		lines = append(lines, "### Body", blankLine)
		lines = append(lines, bodyBlock(response.Body)...)
	}
	return lines
}

func testResultSection(result *script.TestResult) []string {
	if len(result.Tests) == 0 && len(result.Errors) == 0 && len(result.ConsoleLogs) == 0 {
		return nil
	}

	var lines []string
	if len(result.Tests) > 0 {
		passed := 0
		for _, test := range result.Tests {
			if test.Passed {
				passed++
			}
		}
		lines = append(lines, fmt.Sprintf("## Tests (%d/%d passed)", passed, len(result.Tests)), blankLine)
		for _, test := range result.Tests {
			marker := testPassedMarker
			if !test.Passed {
				marker = testFailedMarker
			}
			lines = append(lines, marker+test.Name)
			if test.Error != "" {
				lines = append(lines, "  - "+strings.ReplaceAll(test.Error, "\n", " "))
			}
		}
		lines = append(lines, blankLine)
	}
	if len(result.Errors) > 0 {
		lines = append(lines, "## Script errors", blankLine)
		lines = append(lines, fencedBlock(langText, strings.Join(result.Errors, "\n"))...)
	}
	if len(result.ConsoleLogs) > 0 {
		lines = append(lines, "## Console", blankLine)
		lines = append(lines, fencedBlock(langText, strings.Join(result.ConsoleLogs, "\n"))...)
	}
	return lines
}

// bodyBlock fences a body as pretty-printed JSON when it parses, as XML when
// it looks like markup, and as plain text otherwise.
func bodyBlock(body string) []string {
	var parsed any
	if err := json.Unmarshal([]byte(body), &parsed); err == nil {
		if pretty, err := json.MarshalIndent(parsed, "", "  "); err == nil {
			return fencedBlock(langJSON, string(pretty))
		}
	}
	if strings.HasPrefix(strings.TrimSpace(body), "<") {
		return fencedBlock(langXML, body)
	}
	return fencedBlock(langText, body)
}

// fencedBlock wraps content in a code fence longer than any backtick run in
// it, so a body containing ``` cannot close the block early.
func fencedBlock(lang, content string) []string {
	fence := markdownFence
	for strings.Contains(content, fence) {
		fence += "`"
	}
	return []string{fence + lang, strings.TrimRight(content, "\n"), fence, blankLine}
}
