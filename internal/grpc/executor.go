package grpc

import (
	"context"
	"fmt"
	"strings"

	"google.golang.org/grpc"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/reflect/protodesc"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/descriptorpb"
	"google.golang.org/protobuf/types/dynamicpb"
	rpb "google.golang.org/grpc/reflection/grpc_reflection_v1"
	"postOffice/internal/postman"
)

// ExecuteRequest executes the gRPC request described by req.
// The request URL must use grpc:// or grpcs:// scheme with the format:
//
//	grpc[s]://host:port/ServiceName/MethodName
//
// Returns the response body serialised as JSON.
func ExecuteRequest(ctx context.Context, req *postman.Request) (string, error) {
	endpoint, service, method, tls := ParseGRPCURL(req.URL.Raw)
	if endpoint == "" || service == "" || method == "" {
		return "", fmt.Errorf("invalid gRPC URL %q (expected grpc[s]://host:port/Service/Method)", req.URL.Raw)
	}

	client, err := NewClient(endpoint, tls)
	if err != nil {
		return "", fmt.Errorf("failed to connect to %s: %w", endpoint, err)
	}
	defer client.Close()

	jsonBody := ""
	if req.Body != nil {
		jsonBody = req.Body.Raw
	}

	return client.invoke(ctx, service, method, jsonBody)
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

// invoke calls a single unary gRPC method using server reflection for type information.
func (c *Client) invoke(ctx context.Context, service, method, jsonBody string) (string, error) {
	stub := rpb.NewServerReflectionClient(c.conn)
	stream, err := stub.ServerReflectionInfo(ctx)
	if err != nil {
		return "", fmt.Errorf("failed to open reflection stream: %w", err)
	}
	defer stream.CloseSend()

	files, err := getFileDescriptorsForSymbol(stream, service)
	if err != nil {
		return "", fmt.Errorf("reflection failed for %q: %w", service, err)
	}

	methodDesc, err := findMethodDescriptor(files, service, method)
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
	if err := c.conn.Invoke(ctx, fullMethod, reqMsg, resMsg, grpc.WaitForReady(false)); err != nil {
		return "", fmt.Errorf("gRPC call %q failed: %w", fullMethod, err)
	}

	out, err := protojson.Marshal(resMsg)
	if err != nil {
		return "", fmt.Errorf("failed to marshal response: %w", err)
	}

	return string(out), nil
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
