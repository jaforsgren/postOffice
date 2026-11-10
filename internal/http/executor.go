package http

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptrace"
	"postOffice/internal/postman"
	"postOffice/internal/script"
	"postOffice/internal/timing"
	"strings"
	"time"
)

const (
	requestTimeout     = 30 * time.Second
	defaultContentType = "application/json"
	// SkippedStatus is reported for a request whose pre-request script called
	// pm.execution.skipRequest(); nothing was sent.
	SkippedStatus = "Skipped"
)

var ErrPreRequestScript = errors.New("pre-request script errors")

type Response struct {
	StatusCode     int
	Status         string
	Headers        map[string][]string
	Body           string
	Duration       time.Duration
	Timing         []timing.Phase
	Error          error
	Skipped        bool
	RequestURL     string
	RequestMethod  string
	RequestHeaders map[string]string
	RequestBody    string
}

type Executor struct {
	client *http.Client
}

func NewExecutor() *Executor {
	return &Executor{
		client: &http.Client{
			Timeout: requestTimeout,
		},
	}
}

// Execute sends req. With a non-nil scope it first runs the collection →
// folder → request pre-request scripts (which may rewrite or skip the
// request), and afterwards the test scripts, returning their combined result.
// Without a scope no scripts run and the result is nil.
func (e *Executor) Execute(
	req *postman.Request,
	scope *script.RequestScope,
	variables []postman.VariableSource,
) (*Response, *script.TestResult) {
	return e.ExecuteContext(context.Background(), req, scope, variables)
}

// ExecuteContext is Execute with the HTTP exchange cancelled when ctx is.
// Scripts are not interrupted; they have their own timeout.
func (e *Executor) ExecuteContext(
	ctx context.Context,
	req *postman.Request,
	scope *script.RequestScope,
	variables []postman.VariableSource,
) (*Response, *script.TestResult) {
	start := time.Now()
	resp := &Response{}

	var scriptRun *script.RequestRun
	var scriptResult *script.TestResult
	requestToSend := req
	if scope != nil {
		if scope.Sender == nil {
			scope.Sender = e.SendRequest
		}
		scriptRun = script.NewRequestRun(*scope)
		requestToSend, scriptResult = scriptRun.PreRequest(req)
		if len(scriptResult.Errors) > 0 {
			resp.Error = fmt.Errorf("%w: %v", ErrPreRequestScript, scriptResult.Errors)
			resp.Duration = time.Since(start)
			return resp, scriptResult
		}
		if scriptResult.Skipped {
			resp.Skipped = true
			resp.Status = SkippedStatus
			resp.Duration = time.Since(start)
			return resp, scriptResult
		}
		variables = scope.Variables()
	}

	httpReq, err := e.buildRequest(requestToSend, variables)
	if err != nil {
		resp.Error = err
		resp.Duration = time.Since(start)
		return resp, scriptResult
	}

	recorder := &traceRecorder{}
	httpReq = httpReq.WithContext(httptrace.WithClientTrace(ctx, recorder.clientTrace()))

	resp.RequestMethod = httpReq.Method
	resp.RequestURL = httpReq.URL.String()
	resp.RequestHeaders = make(map[string]string)
	for key, values := range httpReq.Header {
		if len(values) > 0 {
			resp.RequestHeaders[key] = values[0]
		}
	}
	if httpReq.Body != nil {
		bodyBytes, _ := io.ReadAll(httpReq.Body)
		resp.RequestBody = string(bodyBytes)
		httpReq.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))
	}

	// Postman's responseTime covers only the HTTP exchange, not the pre-request scripts.
	sentAt := time.Now()
	httpResp, err := e.client.Do(httpReq)
	if err != nil {
		resp.Error = fmt.Errorf("request failed: %w", err)
		resp.Duration = time.Since(start)
		resp.Timing = recorder.phases(sentAt, time.Now())
		return resp, scriptResult
	}
	defer httpResp.Body.Close()

	resp.StatusCode = httpResp.StatusCode
	resp.Status = httpResp.Status
	resp.Headers = httpResp.Header

	body, err := io.ReadAll(httpResp.Body)
	recorder.markBodyRead()
	resp.Timing = recorder.phases(sentAt, time.Now())
	if err != nil {
		resp.Error = fmt.Errorf("failed to read response body: %w", err)
		resp.Duration = time.Since(start)
		return resp, scriptResult
	}

	resp.Body = string(body)
	responseTime := time.Since(sentAt)
	resp.Duration = time.Since(start)

	if scriptRun != nil {
		responseData := &script.ResponseData{
			StatusCode:   resp.StatusCode,
			Status:       resp.Status,
			Body:         resp.Body,
			Headers:      resp.Headers,
			ResponseTime: responseTime.Milliseconds(),
		}
		scriptResult.Merge(scriptRun.Test(script.NewRequestData(requestToSend), responseData))
	}

	return resp, scriptResult
}

// SendRequest performs a script's pm.sendRequest call. As in Postman, the
// spec is sent as given: {{variables}} are not resolved.
func (e *Executor) SendRequest(spec script.SendRequestSpec) (*script.ResponseData, error) {
	var body io.Reader
	if spec.Body != "" {
		body = strings.NewReader(spec.Body)
	}
	httpReq, err := http.NewRequest(spec.Method, spec.URL, body)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	for _, header := range spec.Headers {
		httpReq.Header.Set(header.Key, header.Value)
	}
	applyDefaultContentType(httpReq, body != nil)

	start := time.Now()
	httpResp, err := e.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer httpResp.Body.Close()

	respBody, err := io.ReadAll(httpResp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	return &script.ResponseData{
		StatusCode:   httpResp.StatusCode,
		Status:       httpResp.Status,
		Body:         string(respBody),
		Headers:      httpResp.Header,
		ResponseTime: time.Since(start).Milliseconds(),
	}, nil
}

func applyDefaultContentType(httpReq *http.Request, hasBody bool) {
	if hasBody && httpReq.Header.Get("Content-Type") == "" {
		httpReq.Header.Set("Content-Type", defaultContentType)
	}
}

func (e *Executor) buildRequest(req *postman.Request, variables []postman.VariableSource) (*http.Request, error) {
	url := req.URL.Raw
	if url == "" {
		url = e.buildURL(&req.URL)
	}
	url = postman.ResolveVariables(url, variables)

	body, multipartContentType, err := buildBody(req.Body, variables)
	if err != nil {
		return nil, err
	}

	httpReq, err := http.NewRequest(req.Method, url, body)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	for _, header := range req.Header {
		resolvedValue := postman.ResolveVariables(header.Value, variables)
		httpReq.Header.Set(header.Key, resolvedValue)
	}

	if multipartContentType != "" {
		// Overrides a hand-set multipart Content-Type: only this one carries the boundary.
		httpReq.Header.Set("Content-Type", multipartContentType)
	}
	applyDefaultContentType(httpReq, body != nil)

	return httpReq, nil
}

func (e *Executor) buildURL(url *postman.URL) string {
	if len(url.Host) == 0 {
		return url.Raw
	}

	var sb strings.Builder
	sb.WriteString("https://")
	sb.WriteString(strings.Join(url.Host, "."))

	if len(url.Path) > 0 {
		sb.WriteString("/")
		sb.WriteString(strings.Join(url.Path, "/"))
	}

	return sb.String()
}
