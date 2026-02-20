package grpc

import (
	"context"
	"fmt"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	rpb "google.golang.org/grpc/reflection/grpc_reflection_v1"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/descriptorpb"
)

// MethodInfo holds reflected metadata about a single gRPC method.
type MethodInfo struct {
	Name          string // "SayHello"
	FullMethod    string // "helloworld.Greeter/SayHello"
	InputType     string // ".helloworld.HelloRequest"
	InputTemplate string // JSON template generated from the input message descriptor
}

// ServiceInfo holds reflected metadata about a single gRPC service.
type ServiceInfo struct {
	Name    string
	Methods []MethodInfo
}

// Client wraps a gRPC connection for reflection queries.
type Client struct {
	conn *grpc.ClientConn
}

// NewClient creates an insecure gRPC client connected to the given endpoint.
func NewClient(endpoint string) (*Client, error) {
	conn, err := grpc.NewClient(endpoint,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create gRPC client for %s: %w", endpoint, err)
	}
	return &Client{conn: conn}, nil
}

// Close releases the underlying connection.
func (c *Client) Close() error {
	return c.conn.Close()
}

type reflectionStream = grpc.BidiStreamingClient[rpb.ServerReflectionRequest, rpb.ServerReflectionResponse]

// ListServices uses server reflection to enumerate all services and their methods,
// and generates a JSON message template for each method's input type.
func (c *Client) ListServices(ctx context.Context) ([]ServiceInfo, error) {
	stub := rpb.NewServerReflectionClient(c.conn)
	stream, err := stub.ServerReflectionInfo(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to open reflection stream: %w", err)
	}
	defer stream.CloseSend()

	serviceNames, err := listServiceNames(stream)
	if err != nil {
		return nil, err
	}

	var services []ServiceInfo
	for _, name := range serviceNames {
		svcInfo, err := describeService(stream, name)
		if err != nil {
			continue
		}
		if svcInfo != nil {
			services = append(services, *svcInfo)
		}
	}

	return services, nil
}

func listServiceNames(stream reflectionStream) ([]string, error) {
	if err := stream.Send(&rpb.ServerReflectionRequest{
		MessageRequest: &rpb.ServerReflectionRequest_ListServices{ListServices: ""},
	}); err != nil {
		return nil, fmt.Errorf("failed to send list_services: %w", err)
	}

	resp, err := stream.Recv()
	if err != nil {
		return nil, fmt.Errorf("failed to receive list_services response: %w", err)
	}

	listResp, ok := resp.MessageResponse.(*rpb.ServerReflectionResponse_ListServicesResponse)
	if !ok {
		if errResp, ok := resp.MessageResponse.(*rpb.ServerReflectionResponse_ErrorResponse); ok {
			return nil, fmt.Errorf("reflection error: %s", errResp.ErrorResponse.ErrorMessage)
		}
		return nil, fmt.Errorf("unexpected response type from list_services")
	}

	var names []string
	for _, svc := range listResp.ListServicesResponse.Service {
		if svc.Name == "grpc.reflection.v1alpha.ServerReflection" ||
			svc.Name == "grpc.reflection.v1.ServerReflection" {
			continue
		}
		names = append(names, svc.Name)
	}
	return names, nil
}

func describeService(stream reflectionStream, serviceName string) (*ServiceInfo, error) {
	if err := stream.Send(&rpb.ServerReflectionRequest{
		MessageRequest: &rpb.ServerReflectionRequest_FileContainingSymbol{
			FileContainingSymbol: serviceName,
		},
	}); err != nil {
		return nil, fmt.Errorf("failed to send file_containing_symbol for %s: %w", serviceName, err)
	}

	resp, err := stream.Recv()
	if err != nil {
		return nil, fmt.Errorf("failed to receive descriptor for %s: %w", serviceName, err)
	}

	fdResp, ok := resp.MessageResponse.(*rpb.ServerReflectionResponse_FileDescriptorResponse)
	if !ok {
		return nil, nil
	}

	files, err := unmarshalFileDescriptors(fdResp.FileDescriptorResponse.FileDescriptorProto)
	if err != nil {
		return nil, err
	}

	return buildServiceInfo(files, serviceName), nil
}

func unmarshalFileDescriptors(rawDescriptors [][]byte) ([]*descriptorpb.FileDescriptorProto, error) {
	files := make([]*descriptorpb.FileDescriptorProto, 0, len(rawDescriptors))
	for _, raw := range rawDescriptors {
		var fd descriptorpb.FileDescriptorProto
		if err := proto.Unmarshal(raw, &fd); err != nil {
			return nil, fmt.Errorf("failed to unmarshal file descriptor: %w", err)
		}
		files = append(files, &fd)
	}
	return files, nil
}

func buildServiceInfo(files []*descriptorpb.FileDescriptorProto, serviceName string) *ServiceInfo {
	for _, fd := range files {
		pkg := fd.GetPackage()
		for _, svcDesc := range fd.GetService() {
			fullName := svcDesc.GetName()
			if pkg != "" {
				fullName = pkg + "." + svcDesc.GetName()
			}
			if fullName != serviceName {
				continue
			}

			svc := ServiceInfo{Name: serviceName}
			for _, method := range svcDesc.GetMethod() {
				template := BuildMessageTemplate(files, method.GetInputType())
				svc.Methods = append(svc.Methods, MethodInfo{
					Name:          method.GetName(),
					FullMethod:    serviceName + "/" + method.GetName(),
					InputType:     method.GetInputType(),
					InputTemplate: template,
				})
			}
			return &svc
		}
	}
	return nil
}
