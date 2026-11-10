package grpc

import (
	"context"
	"fmt"
	"strings"
	"time"

	"google.golang.org/grpc"
	rpb "google.golang.org/grpc/reflection/grpc_reflection_v1"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/reflect/protodesc"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/descriptorpb"
	"google.golang.org/protobuf/types/dynamicpb"
	"postOffice/internal/postman"
	"postOffice/internal/timing"
)

const (
	// PhaseReflect covers connection setup as well, because the client dials lazily on the first RPC.
	PhaseReflect = "Connect + reflection"
	PhaseInvoke  = "Invoke"
)

// Result is the outcome of a gRPC call. Phases and Duration are populated even when the call fails.
type Result struct {
	Body     string
	Phases   []timing.Phase
	Duration time.Duration
}

// ExecuteRequest executes the gRPC request described by req.
// The request URL must use grpc:// or grpcs:// scheme with the format:
//
//	grpc[s]://host:port/ServiceName/MethodName
//
// The response body is serialised as JSON.
func ExecuteRequest(ctx context.Context, req *postman.Request) (*Result, error) {
	start := time.Now()
	sequence := timing.NewSequence(start)
	result := &Result{}
	defer func() {
		result.Phases = sequence.Phases()
		result.Duration = time.Since(start)
	}()

	endpoint, service, method, tls := ParseGRPCURL(req.URL.Raw)
	if endpoint == "" || service == "" || method == "" {
		return result, fmt.Errorf("invalid gRPC URL %q (expected grpc[s]://host:port/Service/Method)", req.URL.Raw)
	}

	client, err := NewClient(endpoint, tls)
	if err != nil {
		return result, fmt.Errorf("failed to connect to %s: %w", endpoint, err)
	}
	defer client.Close()

	jsonBody := ""
	if req.Body != nil {
		jsonBody = req.Body.Raw
	}

	result.Body, err = client.invoke(ctx, service, method, jsonBody, sequence)
	return result, err
}

// ParseGRPCURL splits a grpc:// or grpcs:// URL into its components.
// Returns empty strings when the URL does not match the expected format.
func ParseGRPCURL(rawURL string) (endpoint, service, method string, tlsEnabled bool) {
	lower := strings.ToLower(rawURL)
	if strings.HasPrefix(lower, "grpcs://") {
		tlsEnabled = true
		rawURL = rawURL[len("grpcs://"):]
	} else if strings.HasPrefix(lower, "grpc://") {
		rawURL = rawURL[len("grpc://"):]
	} else {
		return
	}

	// rawURL is now "host:port/Service/Method"
	slashIdx := strings.Index(rawURL, "/")
	if slashIdx < 0 {
		endpoint = rawURL
		return
	}

	endpoint = rawURL[:slashIdx]
	rest := rawURL[slashIdx+1:]

	lastSlash := strings.LastIndex(rest, "/")
	if lastSlash < 0 {
		service = rest
		return
	}

	service = rest[:lastSlash]
	method = rest[lastSlash+1:]
	return
}

// invoke calls a single unary gRPC method using server reflection for type information,
// marking the reflection and invocation phases on sequence.
func (c *Client) invoke(ctx context.Context, service, method, jsonBody string, sequence *timing.Sequence) (string, error) {
	methodDesc, err := c.resolveMethod(ctx, service, method)
	sequence.Mark(PhaseReflect)
	if err != nil {
		return "", err
	}

	reqMsg := dynamicpb.NewMessage(methodDesc.Input())
	if strings.TrimSpace(jsonBody) != "" {
		if err := protojson.Unmarshal([]byte(jsonBody), reqMsg); err != nil {
			return "", fmt.Errorf("failed to unmarshal request body: %w", err)
		}
	}

	resMsg := dynamicpb.NewMessage(methodDesc.Output())

	fullMethod := "/" + service + "/" + method
	err = c.conn.Invoke(ctx, fullMethod, reqMsg, resMsg, grpc.WaitForReady(false))
	sequence.Mark(PhaseInvoke)
	if err != nil {
		return "", fmt.Errorf("gRPC call %q failed: %w", fullMethod, err)
	}

	out, err := protojson.Marshal(resMsg)
	if err != nil {
		return "", fmt.Errorf("failed to marshal response: %w", err)
	}

	return string(out), nil
}

// resolveMethod looks up the method descriptor for service/method via server reflection.
func (c *Client) resolveMethod(ctx context.Context, service, method string) (protoreflect.MethodDescriptor, error) {
	stub := rpb.NewServerReflectionClient(c.conn)
	stream, err := stub.ServerReflectionInfo(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to open reflection stream: %w", err)
	}
	defer stream.CloseSend()

	files, err := getFileDescriptorsForSymbol(stream, service)
	if err != nil {
		return nil, fmt.Errorf("reflection failed for %q: %w", service, err)
	}

	return findMethodDescriptor(files, service, method)
}

// getFileDescriptorsForSymbol fetches all FileDescriptorProtos that define the
// given symbol (and their transitive dependencies) via server reflection.
func getFileDescriptorsForSymbol(stream reflectionStream, symbol string) ([]*descriptorpb.FileDescriptorProto, error) {
	if err := stream.Send(&rpb.ServerReflectionRequest{
		MessageRequest: &rpb.ServerReflectionRequest_FileContainingSymbol{
			FileContainingSymbol: symbol,
		},
	}); err != nil {
		return nil, fmt.Errorf("failed to send file_containing_symbol for %q: %w", symbol, err)
	}

	resp, err := stream.Recv()
	if err != nil {
		return nil, fmt.Errorf("failed to receive descriptor for %q: %w", symbol, err)
	}

	fdResp, ok := resp.MessageResponse.(*rpb.ServerReflectionResponse_FileDescriptorResponse)
	if !ok {
		if errResp, ok2 := resp.MessageResponse.(*rpb.ServerReflectionResponse_ErrorResponse); ok2 {
			return nil, fmt.Errorf("reflection error: %s", errResp.ErrorResponse.ErrorMessage)
		}
		return nil, fmt.Errorf("unexpected reflection response type for %q", symbol)
	}

	return unmarshalFileDescriptors(fdResp.FileDescriptorResponse.FileDescriptorProto)
}

// findMethodDescriptor locates the method descriptor for service/method
// within the given set of file descriptors.
func findMethodDescriptor(
	files []*descriptorpb.FileDescriptorProto,
	serviceName, methodName string,
) (protoreflect.MethodDescriptor, error) {
	fdSet := &descriptorpb.FileDescriptorSet{File: files}
	registry, err := protodesc.NewFiles(fdSet)
	if err != nil {
		return nil, fmt.Errorf("failed to build proto registry: %w", err)
	}

	desc, err := registry.FindDescriptorByName(protoreflect.FullName(serviceName))
	if err != nil {
		return nil, fmt.Errorf("service %q not found in file descriptors: %w", serviceName, err)
	}

	svcDesc, ok := desc.(protoreflect.ServiceDescriptor)
	if !ok {
		return nil, fmt.Errorf("%q is not a service descriptor", serviceName)
	}

	methodDesc := svcDesc.Methods().ByName(protoreflect.Name(methodName))
	if methodDesc == nil {
		return nil, fmt.Errorf("method %q not found in service %q", methodName, serviceName)
	}

	return methodDesc, nil
}
