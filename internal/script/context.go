package script

import (
	"slices"

	"postOffice/internal/postman"
)

// Phase names the Postman event a script listens to.
type Phase string

const (
	PhasePreRequest Phase = "prerequest"
	PhaseTest       Phase = "test"
)

type ResponseData struct {
	StatusCode   int
	Status       string
	Body         string
	Headers      map[string][]string
	ResponseTime int64
}

// RequestData is the mutable view of the outgoing request that pm.request
// exposes. Values are unresolved ({{var}} placeholders intact), as in Postman.
type RequestData struct {
	Method  string
	URL     string
	Headers []postman.Header
	Body    string
}

// RequestInfo backs pm.info.
type RequestInfo struct {
	RequestName string
	RequestID   string
}

// SendRequestSpec is what a script passed to pm.sendRequest, already
// normalised from Postman's string/object forms.
type SendRequestSpec struct {
	Method  string
	URL     string
	Headers []postman.Header
	Body    string
}

// RequestSender performs a pm.sendRequest call. It lives outside this package
// (internal/http) because the http package already depends on script.
type RequestSender func(spec SendRequestSpec) (*ResponseData, error)

type ExecutionContext struct {
	Response        *ResponseData
	Request         *RequestData
	Info            RequestInfo
	Iteration       Iteration
	EnvironmentName string
	SendRequest     RequestSender
	CollectionVars  []postman.Variable
	EnvironmentVars []postman.EnvVariable
	// FolderVars are the variables of the folders above the request,
	// outermost first. Scripts read them but cannot set them.
	FolderVars []postman.VariableSource
}

// NextRequest records a pm.execution.setNextRequest call. Name is empty with
// Set=true when the script passed null, which means "stop the run".
type NextRequest struct {
	Set  bool
	Name string
}

type TestResult struct {
	Tests       []Test
	Errors      []string
	ConsoleLogs []string
	Skipped     bool
	NextRequest NextRequest
}

type Test struct {
	Name   string
	Passed bool
	Error  string
}

func newTestResult() *TestResult {
	return &TestResult{
		Tests:  []Test{},
		Errors: []string{},
	}
}

func (tr *TestResult) AddTest(name string, passed bool, err string) {
	tr.Tests = append(tr.Tests, Test{
		Name:   name,
		Passed: passed,
		Error:  err,
	})
}

func (tr *TestResult) AddError(err string) {
	tr.Errors = append(tr.Errors, err)
}

func (tr *TestResult) AddLog(msg string) {
	tr.ConsoleLogs = append(tr.ConsoleLogs, msg)
}

// Merge appends other's tests, errors and logs, and carries over skip and
// next-request decisions. A nil other is a no-op.
func (tr *TestResult) Merge(other *TestResult) {
	if other == nil {
		return
	}
	tr.Tests = append(tr.Tests, other.Tests...)
	tr.Errors = append(tr.Errors, other.Errors...)
	tr.ConsoleLogs = append(tr.ConsoleLogs, other.ConsoleLogs...)
	tr.Skipped = tr.Skipped || other.Skipped
	if other.NextRequest.Set {
		tr.NextRequest = other.NextRequest
	}
}

func (tr *TestResult) HasFailures() bool {
	for _, test := range tr.Tests {
		if !test.Passed {
			return true
		}
	}
	return len(tr.Errors) > 0
}

// NewRequestData snapshots req for scripts to read and mutate. A nil req
// yields nil, so pm.request is simply absent.
func NewRequestData(req *postman.Request) *RequestData {
	if req == nil {
		return nil
	}
	data := &RequestData{
		Method:  req.Method,
		URL:     req.URL.Raw,
		Headers: slices.Clone(req.Header),
	}
	if req.Body != nil {
		data.Body = req.Body.Raw
	}
	return data
}

// ApplyTo returns a copy of req with the script-visible fields replaced by
// rd's (possibly script-mutated) values. req itself is never modified, since
// it usually belongs to the loaded collection.
func (rd *RequestData) ApplyTo(req *postman.Request) *postman.Request {
	if rd == nil || req == nil {
		return req
	}
	updated := *req
	updated.Method = rd.Method
	updated.Header = slices.Clone(rd.Headers)
	if rd.URL != req.URL.Raw {
		updated.URL = postman.URL{Raw: rd.URL}
	}
	switch {
	case req.Body != nil:
		body := *req.Body
		body.Raw = rd.Body
		updated.Body = &body
	case rd.Body != "":
		updated.Body = &postman.Body{Mode: "raw", Raw: rd.Body}
	}
	return &updated
}
