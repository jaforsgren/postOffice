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
}

func NewExecutor() *Executor {
	return &Executor{timeout: requestTimeout}
}

// Execute resolves req against variables, runs item's scripts around the call
// and returns the response in the same shape as an HTTP execution. A nil item
// runs no scripts.
func (e *Executor) Execute(
	req *postman.Request,
	item *postman.Item,
	collection *postman.Collection,
	environment *postman.Environment,
	variables []postman.VariableSource,
) (*httpexec.Response, *script.TestResult) {
	start := time.Now()
	resp := &httpexec.Response{RequestMethod: requestMethod}

	if item != nil {
		if errors := script.RunItemPreRequestScripts(item, collection, environment); len(errors) > 0 {
			resp.Error = fmt.Errorf("pre-request script errors: %v", errors)
			resp.Duration = time.Since(start)
			return resp, nil
		}
		variables = postman.GetAllVariables(collection, nil, environment)
	}

	resolved := resolveRequest(req, variables)
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

	body, err := ExecuteRequest(ctx, resolved, variableValue(variables, ProtosetVariable))
	resp.Duration = time.Since(start)
	if err != nil {
		resp.Error = err
		resp.Status = status.Code(err).String()
		return resp, nil
	}

	resp.StatusCode = okStatusCode
	resp.Status = status.Code(nil).String()
	resp.Body = body

	testResult := script.RunItemTestScripts(item, collection, environment, &script.ResponseData{
		StatusCode:   resp.StatusCode,
		Status:       resp.Status,
		Body:         resp.Body,
		ResponseTime: resp.Duration.Milliseconds(),
	})
	return resp, testResult
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
