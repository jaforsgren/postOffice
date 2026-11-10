package grpc

import (
	"context"
	"fmt"
	"time"

	"google.golang.org/grpc/status"
	httpexec "postOffice/internal/http"
	"postOffice/internal/postman"
	"postOffice/internal/script"
)

const (
	requestTimeout = 30 * time.Second
	requestMethod  = "GRPC"
	// ProtosetVariable names the variable holding the descriptor set used for
	// servers without reflection.
	ProtosetVariable = "GRPC_PROTOSET"
	// A successful call is reported as HTTP 200 so Postman-style tests such as
	// pm.response.to.have.status(200) hold for gRPC requests too.
	okStatusCode = 200
)

// Executor runs gRPC requests the way http.Executor runs HTTP ones: variables
// resolved, pre-request scripts before the call and test scripts after it.
type Executor struct {
	timeout time.Duration
	// pm.sendRequest in a gRPC request's scripts still sends HTTP.
	sender script.RequestSender
}

func NewExecutor() *Executor {
	return &Executor{timeout: requestTimeout, sender: httpexec.NewExecutor().SendRequest}
}

// Execute resolves req against variables and calls it. With a non-nil scope
// the collection → folder → request pre-request scripts run first (and may
// rewrite or skip the request), and the test scripts after a successful call.
// Without a scope no scripts run and the result is nil.
func (e *Executor) Execute(
	req *postman.Request,
	scope *script.RequestScope,
	variables []postman.VariableSource,
) (*httpexec.Response, *script.TestResult) {
	start := time.Now()
	resp := &httpexec.Response{RequestMethod: requestMethod}

	var scriptRun *script.RequestRun
	var scriptResult *script.TestResult
	requestToSend := req
	if scope != nil {
		if scope.Sender == nil {
			scope.Sender = e.sender
		}
		scriptRun = script.NewRequestRun(*scope)
		requestToSend, scriptResult = scriptRun.PreRequest(req)
		if len(scriptResult.Errors) > 0 {
			resp.Error = fmt.Errorf("%w: %v", httpexec.ErrPreRequestScript, scriptResult.Errors)
			resp.Duration = time.Since(start)
			return resp, scriptResult
		}
		if scriptResult.Skipped {
			resp.Skipped = true
			resp.Status = httpexec.SkippedStatus
			resp.Duration = time.Since(start)
			return resp, scriptResult
		}
		variables = scope.Variables()
	}

	resolved := resolveRequest(requestToSend, variables)
	resp.RequestURL = resolved.URL.Raw
	resp.RequestHeaders = make(map[string]string, len(resolved.Header))
	for _, header := range resolved.Header {
		resp.RequestHeaders[header.Key] = header.Value
	}
	if resolved.Body != nil {
		resp.RequestBody = resolved.Body.Raw
	}

	ctx, cancel := context.WithTimeout(context.Background(), e.timeout)
	defer cancel()

	result, err := ExecuteRequest(ctx, resolved, variableValue(variables, ProtosetVariable))
	resp.Duration = time.Since(start)
	resp.Timing = result.Phases
	if err != nil {
		resp.Error = err
		resp.Status = status.Code(err).String()
		return resp, scriptResult
	}

	resp.StatusCode = okStatusCode
	resp.Status = status.Code(nil).String()
	resp.Body = result.Body

	if scriptRun != nil {
		scriptResult.Merge(scriptRun.Test(script.NewRequestData(requestToSend), &script.ResponseData{
			StatusCode:   resp.StatusCode,
			Status:       resp.Status,
			Body:         resp.Body,
			ResponseTime: resp.Duration.Milliseconds(),
		}))
	}
	return resp, scriptResult
}

func resolveRequest(req *postman.Request, variables []postman.VariableSource) *postman.Request {
	resolved := *req
	resolved.URL.Raw = postman.ResolveVariables(req.URL.Raw, variables)
	resolved.Header = make([]postman.Header, len(req.Header))
	for i, header := range req.Header {
		resolved.Header[i] = postman.Header{
			Key:   postman.ResolveVariables(header.Key, variables),
			Value: postman.ResolveVariables(header.Value, variables),
			Type:  header.Type,
		}
	}
	if req.Body != nil {
		resolved.Body = &postman.Body{
			Mode: req.Body.Mode,
			Raw:  postman.ResolveVariables(req.Body.Raw, variables),
		}
	}
	return &resolved
}

func variableValue(variables []postman.VariableSource, key string) string {
	for _, variable := range variables {
		if variable.Key == key {
			return variable.Value
		}
	}
	return ""
}
