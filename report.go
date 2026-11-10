package main

import (
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"strings"

	"postOffice/internal/workflow"
)

// outputFormat selects how a run is reported.
type outputFormat string

const (
	outputText  outputFormat = "text"
	outputJSON  outputFormat = "json"
	outputJUnit outputFormat = "junit"
)

// machineWriters render the machine-readable formats; text is written by writeTextReport.
var machineWriters = map[outputFormat]func(io.Writer, runReport) error{
	outputJSON:  writeJSONReport,
	outputJUnit: writeJUnitReport,
}

// requestTestCaseName names the JUnit testcase that stands for the request itself: it carries
// request failures (HTTP and script errors), skips, and requests that define no tests.
const requestTestCaseName = "request"

const (
	iterationLabelPrefix  = "#"
	jsonIndent            = "  "
	testIndent            = "    "
	millisecondsPerSecond = 1000.0
)

// runReport is the outcome of one headless run, independent of output format.
type runReport struct {
	Name        string          `json:"name"`
	Collection  string          `json:"collection"`
	Environment string          `json:"environment,omitempty"`
	Status      string          `json:"status"`
	Iterations  int             `json:"iterations,omitempty"`
	Passed      bool            `json:"passed"`
	Error       string          `json:"error,omitempty"`
	Summary     reportSummary   `json:"summary"`
	Requests    []requestReport `json:"requests"`
	Logs        []string        `json:"logs,omitempty"`
}

type reportSummary struct {
	Requests    int `json:"requests"`
	Passed      int `json:"passed"`
	Failed      int `json:"failed"`
	Skipped     int `json:"skipped"`
	Tests       int `json:"tests"`
	TestsPassed int `json:"testsPassed"`
	TestsFailed int `json:"testsFailed"`
}

type requestReport struct {
	Step    string `json:"step"`
	Request string `json:"request"`
	// Iteration is set (1-based) only in runs with more than one iteration.
	Iteration  int          `json:"iteration,omitempty"`
	Status     string       `json:"status"`
	HTTPStatus string       `json:"httpStatus,omitempty"`
	DurationMs int64        `json:"durationMs"`
	Tests      []testReport `json:"tests"`
	Errors     []string     `json:"errors,omitempty"`
	Logs       []string     `json:"logs,omitempty"`
}

type testReport struct {
	Name   string `json:"name"`
	Passed bool   `json:"passed"`
	Error  string `json:"error,omitempty"`
}

// newRunReport summarises a finished run. Steps the run never reached are
// reported as skipped, as the workflow engine marks them. Logs holds the
// workflow script's own output; each request carries its console output.
func newRunReport(state workflow.ExecutionState, collectionName, environmentName string) runReport {
	report := runReport{
		Name:        state.WorkflowName,
		Collection:  collectionName,
		Environment: environmentName,
		Status:      state.Status,
		Error:       state.Error,
		Requests:    []requestReport{},
	}

	multiIteration := state.IsMultiIteration()
	if multiIteration {
		report.Iterations = state.Iterations
	}

	type stepRun struct {
		stepID    string
		iteration int
	}
	stepLogs := map[stepRun][]string{}
	for _, entry := range state.Logs {
		if entry.StepID == "" {
			report.Logs = append(report.Logs, entry.Message)
		} else {
			key := stepRun{entry.StepID, entry.Iteration}
			stepLogs[key] = append(stepLogs[key], entry.Message)
		}
	}

	for _, step := range state.Steps {
		request := requestReport{Step: step.ID, Request: step.Request, Status: step.Status, Tests: []testReport{}}
		if multiIteration {
			request.Iteration = step.Iteration
		}
		if resp := step.LastResponse; resp != nil {
			request.HTTPStatus = resp.Status
			request.DurationMs = resp.Duration.Milliseconds()
			if resp.Error != nil {
				request.Errors = append(request.Errors, resp.Error.Error())
			}
		}
		if result := step.LastResult; result != nil {
			for _, test := range result.Tests {
				request.Tests = append(request.Tests, testReport{Name: test.Name, Passed: test.Passed, Error: test.Error})
			}
			request.Errors = append(request.Errors, result.Errors...)
			request.Logs = result.ConsoleLogs
		} else if step.Status == workflow.StatusFailed {
			// The step failed before its request ran (unresolvable path, failing
			// step script), so the engine's log is the only explanation.
			request.Errors = append(request.Errors, stepLogs[stepRun{step.ID, step.Iteration}]...)
		}
		report.Requests = append(report.Requests, request)
		report.Summary.add(request)
	}

	report.Passed = report.Summary.Failed == 0 && state.Status != workflow.StatusFailed
	return report
}

func (s *reportSummary) add(request requestReport) {
	s.Requests++
	switch request.Status {
	case workflow.StatusSuccess:
		s.Passed++
	case workflow.StatusSkipped:
		s.Skipped++
	default:
		s.Failed++
	}
	for _, test := range request.Tests {
		s.Tests++
		if test.Passed {
			s.TestsPassed++
		} else {
			s.TestsFailed++
		}
	}
}

// label names the request in text and JUnit output, prefixed with "#n " in
// multi-iteration runs so each iteration's run of a request stays apart.
func (r requestReport) label() string {
	if r.Iteration == 0 {
		return r.Request
	}
	return fmt.Sprintf("%s%d %s", iterationLabelPrefix, r.Iteration, r.Request)
}

func writeTextReport(out io.Writer, report runReport) {
	fmt.Fprintf(out, "Run %q (collection %s", report.Name, report.Collection)
	if report.Environment != "" {
		fmt.Fprintf(out, ", environment %s", report.Environment)
	}
	fmt.Fprint(out, ")\n\n")

	for _, request := range report.Requests {
		fmt.Fprintf(out, "%s %s", workflow.StatusIcon(request.Status), request.label())
		if request.HTTPStatus != "" {
			fmt.Fprintf(out, "  %s  %dms", request.HTTPStatus, request.DurationMs)
		} else {
			fmt.Fprintf(out, "  %s", request.Status)
		}
		fmt.Fprintln(out)
		for _, test := range request.Tests {
			if test.Passed {
				fmt.Fprintf(out, "%s%s %s\n", testIndent, workflow.StatusIcon(workflow.StatusSuccess), test.Name)
			} else {
				fmt.Fprintf(out, "%s%s %s: %s\n", testIndent, workflow.StatusIcon(workflow.StatusFailed), test.Name, test.Error)
			}
		}
		for _, requestErr := range request.Errors {
			fmt.Fprintf(out, "%serror: %s\n", testIndent, requestErr)
		}
		for _, log := range request.Logs {
			fmt.Fprintf(out, "%s%s\n", testIndent, log)
		}
	}

	s := report.Summary
	fmt.Fprintln(out)
	if report.Iterations > 0 {
		fmt.Fprintf(out, "%d iterations\n", report.Iterations)
	}
	fmt.Fprintf(out, "%d requests: %d passed, %d failed, %d skipped\n", s.Requests, s.Passed, s.Failed, s.Skipped)
	fmt.Fprintf(out, "%d tests: %d passed, %d failed\n", s.Tests, s.TestsPassed, s.TestsFailed)
	if report.Passed {
		fmt.Fprintln(out, "PASSED")
	} else {
		fmt.Fprintf(out, "FAILED: %s\n", report.Error)
	}
}

func writeJSONReport(out io.Writer, report runReport) error {
	encoder := json.NewEncoder(out)
	encoder.SetIndent("", jsonIndent)
	return encoder.Encode(report)
}

type junitTestSuites struct {
	XMLName  xml.Name         `xml:"testsuites"`
	Name     string           `xml:"name,attr"`
	Tests    int              `xml:"tests,attr"`
	Failures int              `xml:"failures,attr"`
	Skipped  int              `xml:"skipped,attr"`
	Time     string           `xml:"time,attr"`
	Suites   []junitTestSuite `xml:"testsuite"`
}

type junitTestSuite struct {
	Name     string          `xml:"name,attr"`
	Tests    int             `xml:"tests,attr"`
	Failures int             `xml:"failures,attr"`
	Skipped  int             `xml:"skipped,attr"`
	Time     string          `xml:"time,attr"`
	Cases    []junitTestCase `xml:"testcase"`
}

type junitTestCase struct {
	Name      string        `xml:"name,attr"`
	ClassName string        `xml:"classname,attr"`
	Time      string        `xml:"time,attr"`
	Failure   *junitFailure `xml:"failure,omitempty"`
	Skipped   *struct{}     `xml:"skipped,omitempty"`
}

type junitFailure struct {
	Message string `xml:"message,attr"`
	Text    string `xml:",chardata"`
}

// writeJUnitReport writes one testsuite per request with one testcase per
// pm.test, plus a "request" testcase for request-level failures and skips.
func writeJUnitReport(out io.Writer, report runReport) error {
	suites := junitTestSuites{Name: report.Name}
	var totalMs int64
	for _, request := range report.Requests {
		suite := junitTestSuite{Name: request.label(), Time: junitSeconds(request.DurationMs)}
		for _, test := range request.Tests {
			testCase := junitTestCase{Name: test.Name, ClassName: request.label(), Time: junitSeconds(0)}
			if !test.Passed {
				testCase.Failure = &junitFailure{Message: test.Error, Text: test.Error}
			}
			suite.Cases = append(suite.Cases, testCase)
		}
		if requestCase, ok := requestTestCase(request); ok {
			suite.Cases = append(suite.Cases, requestCase)
		}
		for _, testCase := range suite.Cases {
			suite.Tests++
			if testCase.Failure != nil {
				suite.Failures++
			}
			if testCase.Skipped != nil {
				suite.Skipped++
			}
		}
		suites.Tests += suite.Tests
		suites.Failures += suite.Failures
		suites.Skipped += suite.Skipped
		totalMs += request.DurationMs
		suites.Suites = append(suites.Suites, suite)
	}
	suites.Time = junitSeconds(totalMs)

	if _, err := io.WriteString(out, xml.Header); err != nil {
		return err
	}
	encoder := xml.NewEncoder(out)
	encoder.Indent("", jsonIndent)
	if err := encoder.Encode(suites); err != nil {
		return err
	}
	_, err := io.WriteString(out, "\n")
	return err
}

// requestTestCase returns the testcase standing for the request itself, when
// there is something to report beyond its pm.tests.
func requestTestCase(request requestReport) (junitTestCase, bool) {
	testCase := junitTestCase{Name: requestTestCaseName, ClassName: request.label(), Time: junitSeconds(request.DurationMs)}
	switch {
	case request.Status == workflow.StatusSkipped:
		testCase.Skipped = &struct{}{}
	case len(request.Errors) > 0:
		message := request.Errors[0]
		testCase.Failure = &junitFailure{Message: message, Text: strings.Join(request.Errors, "\n")}
	case request.Status == workflow.StatusFailed && len(request.Tests) == 0:
		testCase.Failure = &junitFailure{Message: request.HTTPStatus, Text: request.HTTPStatus}
	case len(request.Tests) > 0:
		return junitTestCase{}, false
	}
	return testCase, true
}

func junitSeconds(ms int64) string {
	return fmt.Sprintf("%.3f", float64(ms)/millisecondsPerSecond)
}
