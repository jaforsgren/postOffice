package http

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"postOffice/internal/postman"
	"strings"
	"time"
)

type Response struct {
	StatusCode int
	Status     string
	Headers    map[string][]string
	Body       string
	Duration   time.Duration
	Error      error
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

func (e *Executor) Execute(req *postman.Request, variables []postman.VariableSource) *Response {
	start := time.Now()
	resp := &Response{}

	httpReq, err := e.buildRequest(req, variables)
	if err != nil {
		resp.Error = err
		resp.Duration = time.Since(start)
		return resp
	}

	httpResp, err := e.client.Do(httpReq)
	if err != nil {
		resp.Error = fmt.Errorf("request failed: %w", err)
		resp.Duration = time.Since(start)
		return resp
	}
	defer httpResp.Body.Close()

	resp.StatusCode = httpResp.StatusCode
	resp.Status = httpResp.Status
	resp.Headers = httpResp.Header

	body, err := io.ReadAll(httpResp.Body)
	if err != nil {
		resp.Error = fmt.Errorf("failed to read response body: %w", err)
		resp.Duration = time.Since(start)
		return resp
	}

	resp.Body = string(body)
	resp.Duration = time.Since(start)

	return resp
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
