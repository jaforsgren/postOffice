package http

import (
	"net/http"
	"net/http/httptest"
	"postOffice/internal/postman"
	"strings"
	"testing"
	"time"
)

func TestNewExecutor(t *testing.T) {
	executor := NewExecutor()

	if executor == nil {
		t.Fatal("Expected executor to be created")
	}
	if executor.client == nil {
		t.Error("Expected client to be initialized")
	}
	if executor.client.Timeout != 30*time.Second {
		t.Errorf("Expected timeout 30s, got %v", executor.client.Timeout)
	}
}

func TestExecute_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"message":"success"}`))
	}))
	defer server.Close()

	executor := NewExecutor()
	req := &postman.Request{
		Method: "GET",
		URL: postman.URL{
			Raw: server.URL,
		},
		Header: []postman.Header{
			{Key: "Accept", Value: "application/json"},
		},
	}

	resp, _ := executor.Execute(req, nil, nil, nil, nil)

	if resp == nil {
		t.Fatal("Expected response")
	}
	if resp.Error != nil {
		t.Errorf("Expected no error, got %v", resp.Error)
	}
	if resp.StatusCode != 200 {
		t.Errorf("Expected status 200, got %d", resp.StatusCode)
	}
	if !strings.Contains(resp.Body, "success") {
		t.Error("Expected 'success' in response body")
	}
	if resp.Duration <= 0 {
		t.Error("Expected duration to be recorded")
	}
	if resp.RequestMethod != "GET" {
		t.Errorf("Expected GET method, got %s", resp.RequestMethod)
	}
	if resp.RequestURL != server.URL {
		t.Errorf("Expected URL %s, got %s", server.URL, resp.RequestURL)
	}
}

func TestExecute_POST_WithBody(t *testing.T) {
	var receivedBody string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			t.Errorf("Expected POST, got %s", r.Method)
		}
		body := make([]byte, 1024)
		n, _ := r.Body.Read(body)
		receivedBody = string(body[:n])
		w.WriteHeader(http.StatusCreated)
		w.Write([]byte(`{"created":true}`))
	}))
	defer server.Close()

	executor := NewExecutor()
	expectedBody := `{"name":"test","value":123}`
	req := &postman.Request{
		Method: "POST",
		URL: postman.URL{
			Raw: server.URL,
		},
		Header: []postman.Header{
			{Key: "Content-Type", Value: "application/json"},
		},
		Body: &postman.Body{
			Mode: "raw",
			Raw:  expectedBody,
		},
	}

	resp, _ := executor.Execute(req, nil, nil, nil, nil)

	if resp.Error != nil {
		t.Errorf("Expected no error, got %v", resp.Error)
	}
	if resp.StatusCode != 201 {
		t.Errorf("Expected status 201, got %d", resp.StatusCode)
	}
	if !strings.Contains(receivedBody, "test") {
		t.Error("Expected request body to be sent")
	}
	if resp.RequestBody != expectedBody {
		t.Errorf("Expected request body %s, got %s", expectedBody, resp.RequestBody)
	}
}

func TestExecute_WithHeaders(t *testing.T) {
	var receivedHeaders http.Header
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedHeaders = r.Header.Clone()
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	executor := NewExecutor()
	req := &postman.Request{
		Method: "GET",
		URL: postman.URL{
			Raw: server.URL,
		},
		Header: []postman.Header{
			{Key: "X-Custom-Header", Value: "custom-value"},
			{Key: "Authorization", Value: "Bearer token123"},
		},
	}

	resp, _ := executor.Execute(req, nil, nil, nil, nil)

	if resp.Error != nil {
		t.Errorf("Expected no error, got %v", resp.Error)
	}
	if receivedHeaders.Get("X-Custom-Header") != "custom-value" {
		t.Error("Expected custom header to be sent")
	}
	if receivedHeaders.Get("Authorization") != "Bearer token123" {
		t.Error("Expected authorization header to be sent")
	}
	if resp.RequestHeaders["X-Custom-Header"] != "custom-value" {
		t.Error("Expected custom header in response metadata")
	}
}

func TestExecute_WithVariables(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	executor := NewExecutor()
	req := &postman.Request{
		Method: "GET",
		URL: postman.URL{
			Raw: "{{baseUrl}}/api/users",
		},
		Header: []postman.Header{
			{Key: "Authorization", Value: "Bearer {{apiKey}}"},
		},
		Body: &postman.Body{
			Mode: "raw",
			Raw:  `{"key":"{{value}}"}`,
		},
	}

	variables := []postman.VariableSource{
		{Key: "baseUrl", Value: server.URL, Source: "test"},
		{Key: "apiKey", Value: "secret123", Source: "test"},
		{Key: "value", Value: "resolved", Source: "test"},
	}

	resp, _ := executor.Execute(req, nil, nil, nil, variables)

	if resp.Error != nil {
		t.Errorf("Expected no error, got %v", resp.Error)
	}
	if !strings.HasPrefix(resp.RequestURL, server.URL) {
		t.Error("Expected URL variable to be resolved")
	}
	if resp.RequestHeaders["Authorization"] != "Bearer secret123" {
		t.Error("Expected header variable to be resolved")
	}
	if !strings.Contains(resp.RequestBody, "resolved") {
		t.Error("Expected body variable to be resolved")
	}
}

func TestExecute_InvalidURL(t *testing.T) {
	executor := NewExecutor()
	req := &postman.Request{
		Method: "GET",
		URL: postman.URL{
			Raw: "://invalid-url",
		},
	}

	resp, _ := executor.Execute(req, nil, nil, nil, nil)

	if resp.Error == nil {
		t.Error("Expected error for invalid URL")
	}
	if resp.Duration <= 0 {
		t.Error("Expected duration to be recorded even on error")
	}
}

func TestExecute_NetworkError(t *testing.T) {
	executor := NewExecutor()
	req := &postman.Request{
		Method: "GET",
		URL: postman.URL{
			Raw: "http://localhost:99999/nonexistent",
		},
	}

	resp, _ := executor.Execute(req, nil, nil, nil, nil)

	if resp.Error == nil {
		t.Error("Expected network error")
	}
	if !strings.Contains(resp.Error.Error(), "request failed") {
		t.Errorf("Expected 'request failed' in error, got %v", resp.Error)
	}
}

func TestExecute_DifferentMethods(t *testing.T) {
	methods := []string{"GET", "POST", "PUT", "DELETE", "PATCH"}

	for _, method := range methods {
		t.Run(method, func(t *testing.T) {
			var receivedMethod string
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				receivedMethod = r.Method
				w.WriteHeader(http.StatusOK)
			}))
			defer server.Close()

			executor := NewExecutor()
			req := &postman.Request{
				Method: method,
				URL: postman.URL{
					Raw: server.URL,
				},
			}

			resp, _ := executor.Execute(req, nil, nil, nil, nil)

			if resp.Error != nil {
				t.Errorf("Expected no error, got %v", resp.Error)
			}
			if receivedMethod != method {
				t.Errorf("Expected method %s, got %s", method, receivedMethod)
			}
			if resp.RequestMethod != method {
				t.Errorf("Expected request method %s, got %s", method, resp.RequestMethod)
			}
		})
	}
}

func TestExecute_ResponseHeaders(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("X-Custom-Response", "custom-value")
		w.Header().Add("X-Multi", "value1")
		w.Header().Add("X-Multi", "value2")
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	executor := NewExecutor()
	req := &postman.Request{
		Method: "GET",
		URL: postman.URL{
			Raw: server.URL,
		},
	}

	resp, _ := executor.Execute(req, nil, nil, nil, nil)

	if resp.Error != nil {
		t.Errorf("Expected no error, got %v", resp.Error)
	}
	if resp.Headers == nil {
		t.Fatal("Expected headers to be set")
	}
	contentType := resp.Headers["Content-Type"]
	if len(contentType) == 0 || contentType[0] != "application/json" {
		t.Error("Expected Content-Type header")
	}
	customResp := resp.Headers["X-Custom-Response"]
	if len(customResp) == 0 || customResp[0] != "custom-value" {
		t.Error("Expected custom response header")
	}
	if len(resp.Headers["X-Multi"]) != 2 {
		t.Error("Expected multiple values for X-Multi header")
	}
}

func TestExecute_DifferentStatusCodes(t *testing.T) {
	testCases := []struct {
		name       string
		statusCode int
	}{
		{"OK", http.StatusOK},
		{"Created", http.StatusCreated},
		{"BadRequest", http.StatusBadRequest},
		{"Unauthorized", http.StatusUnauthorized},
		{"NotFound", http.StatusNotFound},
		{"InternalServerError", http.StatusInternalServerError},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tc.statusCode)
			}))
			defer server.Close()

			executor := NewExecutor()
			req := &postman.Request{
				Method: "GET",
				URL: postman.URL{
					Raw: server.URL,
				},
			}

			resp, _ := executor.Execute(req, nil, nil, nil, nil)

			if resp.Error != nil {
				t.Errorf("Expected no error, got %v", resp.Error)
			}
			if resp.StatusCode != tc.statusCode {
				t.Errorf("Expected status %d, got %d", tc.statusCode, resp.StatusCode)
			}
		})
	}
}

func TestBuildURL_WithHostAndPath(t *testing.T) {
	executor := NewExecutor()
	url := &postman.URL{
		Host: []string{"api", "example", "com"},
		Path: []string{"v1", "users", "123"},
	}

	result := executor.buildURL(url)

	expected := "https://api.example.com/v1/users/123"
	if result != expected {
		t.Errorf("Expected %s, got %s", expected, result)
	}
}

func TestBuildURL_OnlyHost(t *testing.T) {
	executor := NewExecutor()
	url := &postman.URL{
		Host: []string{"example", "com"},
		Path: []string{},
	}

	result := executor.buildURL(url)

	expected := "https://example.com"
	if result != expected {
		t.Errorf("Expected %s, got %s", expected, result)
	}
}

func TestBuildURL_EmptyHost(t *testing.T) {
	executor := NewExecutor()
	url := &postman.URL{
		Raw:  "https://example.com/test",
		Host: []string{},
		Path: []string{"test"},
	}

	result := executor.buildURL(url)

	if result != url.Raw {
		t.Errorf("Expected fallback to Raw URL %s, got %s", url.Raw, result)
	}
}

func TestBuildURL_ComplexPath(t *testing.T) {
	executor := NewExecutor()
	url := &postman.URL{
		Host: []string{"subdomain", "api", "example", "com"},
		Path: []string{"api", "v2", "resources", "item", "edit"},
	}

	result := executor.buildURL(url)

	expected := "https://subdomain.api.example.com/api/v2/resources/item/edit"
	if result != expected {
		t.Errorf("Expected %s, got %s", expected, result)
	}
}

func TestBuildRequest_WithAllComponents(t *testing.T) {
	executor := NewExecutor()
	req := &postman.Request{
		Method: "POST",
		URL: postman.URL{
			Raw: "https://api.example.com/users",
		},
		Header: []postman.Header{
			{Key: "Content-Type", Value: "application/json"},
			{Key: "X-API-Key", Value: "secret"},
		},
		Body: &postman.Body{
			Mode: "raw",
			Raw:  `{"name":"John"}`,
		},
	}

	httpReq, err := executor.buildRequest(req, nil)

	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}
	if httpReq.Method != "POST" {
		t.Errorf("Expected POST, got %s", httpReq.Method)
	}
	if httpReq.URL.String() != "https://api.example.com/users" {
		t.Errorf("Expected correct URL, got %s", httpReq.URL.String())
	}
	if httpReq.Header.Get("Content-Type") != "application/json" {
		t.Error("Expected Content-Type header")
	}
	if httpReq.Header.Get("X-API-Key") != "secret" {
		t.Error("Expected X-API-Key header")
	}
}

func TestBuildRequest_WithoutRawURL(t *testing.T) {
	executor := NewExecutor()
	req := &postman.Request{
		Method: "GET",
		URL: postman.URL{
			Host: []string{"api", "test", "com"},
			Path: []string{"endpoint"},
		},
	}

	httpReq, err := executor.buildRequest(req, nil)

	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}
	if httpReq.URL.String() != "https://api.test.com/endpoint" {
		t.Errorf("Expected built URL, got %s", httpReq.URL.String())
	}
}

func TestBuildRequest_WithVariables(t *testing.T) {
	executor := NewExecutor()
	req := &postman.Request{
		Method: "GET",
		URL: postman.URL{
			Raw: "{{protocol}}://{{host}}/{{path}}",
		},
		Header: []postman.Header{
			{Key: "Authorization", Value: "Bearer {{token}}"},
		},
		Body: &postman.Body{
			Mode: "raw",
			Raw:  `{"id":"{{userId}}"}`,
		},
	}

	variables := []postman.VariableSource{
		{Key: "protocol", Value: "https", Source: "test"},
		{Key: "host", Value: "api.example.com", Source: "test"},
		{Key: "path", Value: "users", Source: "test"},
		{Key: "token", Value: "abc123", Source: "test"},
		{Key: "userId", Value: "456", Source: "test"},
	}

	httpReq, err := executor.buildRequest(req, variables)

	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}
	if !strings.Contains(httpReq.URL.String(), "api.example.com") {
		t.Error("Expected URL variables to be resolved")
	}
	if httpReq.Header.Get("Authorization") != "Bearer abc123" {
		t.Error("Expected header variables to be resolved")
	}
}

func TestBuildRequest_NoBody(t *testing.T) {
	executor := NewExecutor()
	req := &postman.Request{
		Method: "GET",
		URL: postman.URL{
			Raw: "https://example.com",
		},
	}

	httpReq, err := executor.buildRequest(req, nil)

	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}
	if httpReq.Body != nil {
		t.Error("Expected nil body for GET request without body")
	}
}

func TestBuildRequest_EmptyBody(t *testing.T) {
	executor := NewExecutor()
	req := &postman.Request{
		Method: "POST",
		URL: postman.URL{
			Raw: "https://example.com",
		},
		Body: &postman.Body{
			Mode: "raw",
			Raw:  "",
		},
	}

	httpReq, err := executor.buildRequest(req, nil)

	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}
	if httpReq.Body != nil {
		t.Error("Expected nil body for empty body content")
	}
}

func TestExecute_Timeout(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping timeout test in short mode")
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(35 * time.Second)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	executor := NewExecutor()
	req := &postman.Request{
		Method: "GET",
		URL: postman.URL{
			Raw: server.URL,
		},
	}

	resp, _ := executor.Execute(req, nil, nil, nil, nil)

	if resp.Error == nil {
		t.Error("Expected timeout error")
	}
}

func TestExecute_LargeResponse(t *testing.T) {
	largeBody := strings.Repeat("x", 1024*1024)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(largeBody))
	}))
	defer server.Close()

	executor := NewExecutor()
	req := &postman.Request{
		Method: "GET",
		URL: postman.URL{
			Raw: server.URL,
		},
	}

	resp, _ := executor.Execute(req, nil, nil, nil, nil)

	if resp.Error != nil {
		t.Errorf("Expected no error, got %v", resp.Error)
	}
	if len(resp.Body) != len(largeBody) {
		t.Errorf("Expected body length %d, got %d", len(largeBody), len(resp.Body))
	}
}

func TestExecute_MultipleHeaders(t *testing.T) {
	executor := NewExecutor()
	req := &postman.Request{
		Method: "GET",
		URL: postman.URL{
			Raw: "https://example.com",
		},
		Header: []postman.Header{
			{Key: "Accept", Value: "application/json"},
			{Key: "Accept-Language", Value: "en-US"},
			{Key: "Cache-Control", Value: "no-cache"},
			{Key: "User-Agent", Value: "PostOffice/1.0"},
		},
	}

	httpReq, err := executor.buildRequest(req, nil)

	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}
	if len(httpReq.Header) < 4 {
		t.Error("Expected all headers to be set")
	}
	if httpReq.Header.Get("Accept") != "application/json" {
		t.Error("Expected Accept header")
	}
	if httpReq.Header.Get("User-Agent") != "PostOffice/1.0" {
		t.Error("Expected User-Agent header")
	}
}

func TestExecute_EmptyResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	executor := NewExecutor()
	req := &postman.Request{
		Method: "DELETE",
		URL: postman.URL{
			Raw: server.URL,
		},
	}

	resp, _ := executor.Execute(req, nil, nil, nil, nil)

	if resp.Error != nil {
		t.Errorf("Expected no error, got %v", resp.Error)
	}
	if resp.StatusCode != 204 {
		t.Errorf("Expected status 204, got %d", resp.StatusCode)
	}
	if resp.Body != "" {
		t.Error("Expected empty body")
	}
}

func TestResponse_Fields(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("test response"))
	}))
	defer server.Close()

	executor := NewExecutor()
	req := &postman.Request{
		Method: "POST",
		URL: postman.URL{
			Raw: server.URL + "/test",
		},
		Header: []postman.Header{
			{Key: "X-Test", Value: "test-value"},
		},
		Body: &postman.Body{
			Mode: "raw",
			Raw:  "test body",
		},
	}

	resp, _ := executor.Execute(req, nil, nil, nil, nil)

	if resp.StatusCode != 200 {
		t.Errorf("Expected StatusCode 200, got %d", resp.StatusCode)
	}
	if resp.Status == "" {
		t.Error("Expected Status to be set")
	}
	if resp.Headers == nil {
		t.Error("Expected Headers to be set")
	}
	if resp.Body != "test response" {
		t.Errorf("Expected Body 'test response', got '%s'", resp.Body)
	}
	if resp.Duration <= 0 {
		t.Error("Expected Duration to be positive")
	}
	if resp.RequestURL == "" {
		t.Error("Expected RequestURL to be set")
	}
	if resp.RequestMethod != "POST" {
		t.Errorf("Expected RequestMethod 'POST', got '%s'", resp.RequestMethod)
	}
	if len(resp.RequestHeaders) == 0 {
		t.Error("Expected RequestHeaders to be set")
	}
	if resp.RequestBody != "test body" {
		t.Errorf("Expected RequestBody 'test body', got '%s'", resp.RequestBody)
	}
}
