package http

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"postOffice/internal/postman"
	"postOffice/internal/script"
	"strings"
	"time"
)

type Response struct {
	StatusCode     int
	Status         string
	Headers        map[string][]string
	Body           string
	Duration       time.Duration
	Error          error
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
			Timeout: 30 * time.Second,
		},
	}
}

func (e *Executor) Execute(
	req *postman.Request,
	item *postman.Item,
	collection *postman.Collection,
	environment *postman.Environment,
	variables []postman.VariableSource,
) (*Response, *script.TestResult) {
	start := time.Now()
	resp := &Response{}

	httpReq, err := e.buildRequest(req, variables)
	if err != nil {
		resp.Error = err
		resp.Duration = time.Since(start)
		return resp, nil
	}

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

	httpResp, err := e.client.Do(httpReq)
	if err != nil {
		resp.Error = fmt.Errorf("request failed: %w", err)
		resp.Duration = time.Since(start)
		return resp, nil
	}
	defer httpResp.Body.Close()

	resp.StatusCode = httpResp.StatusCode
	resp.Status = httpResp.Status
	resp.Headers = httpResp.Header

	body, err := io.ReadAll(httpResp.Body)
	if err != nil {
		resp.Error = fmt.Errorf("failed to read response body: %w", err)
		resp.Duration = time.Since(start)
		return resp, nil
	}

	resp.Body = string(body)
	resp.Duration = time.Since(start)

	testResult := e.executeTestScripts(item, collection, environment, resp)

	return resp, testResult
}

func (e *Executor) executeTestScripts(
	item *postman.Item,
	collection *postman.Collection,
	environment *postman.Environment,
	resp *Response,
) *script.TestResult {
	if item == nil {
		return nil
	}

	responseData := &script.ResponseData{
		StatusCode: resp.StatusCode,
		Body:       resp.Body,
	}

	ctx := &script.ExecutionContext{
		Response: responseData,
	}

	if collection != nil {
		ctx.CollectionVars = collection.Variables
	}

	if environment != nil {
		ctx.EnvironmentVars = environment.Values
	}

	result := script.ExecuteTestScripts(item.Events, ctx)

	if collection != nil {
		collection.Variables = ctx.CollectionVars
	}

	if environment != nil {
		environment.Values = ctx.EnvironmentVars
	}

	return result
}

func (e *Executor) buildRequest(req *postman.Request, variables []postman.VariableSource) (*http.Request, error) {
	url := req.URL.Raw
	if url == "" {
		url = e.buildURL(&req.URL)
	}
	url = postman.ResolveVariables(url, variables)

	var body io.Reader
	if req.Body != nil && req.Body.Raw != "" {
		resolvedBody := postman.ResolveVariables(req.Body.Raw, variables)
		body = bytes.NewBufferString(resolvedBody)
	}

	httpReq, err := http.NewRequest(req.Method, url, body)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	for _, header := range req.Header {
		resolvedValue := postman.ResolveVariables(header.Value, variables)
		httpReq.Header.Set(header.Key, resolvedValue)
	}

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
