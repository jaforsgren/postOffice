package grpc

import (
	"context"
	"encoding/base64"
	"errors"
	"net"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/grpc/health"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/reflection"
	rpbalpha "google.golang.org/grpc/reflection/grpc_reflection_v1alpha"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protodesc"
	"google.golang.org/protobuf/types/descriptorpb"
	"postOffice/internal/postman"
	"postOffice/internal/script"
)

const (
	healthCheckPath = "/grpc.health.v1.Health/Check"
	callerKey       = "x-caller-info-bin"
	callerValue     = "caller"
	servingService  = "claims"
	unknownService  = "missing"
	// defaultStatusService reports the zero status, which protobuf JSON omits.
	defaultStatusService = "idle"
	statusVariable       = "health_status"
	serviceVariable      = "service_name"
	callerKeyVar         = "callerkey"
	callerValueVar       = "callerVal"
	expectedNotFound     = "NotFound"
)

// healthServer serves grpc.health.v1 with reflection on a local port and
// records the metadata of the last call.
type healthServer struct {
	endpoint string
	mu       sync.Mutex
	metadata metadata.MD
}

type reflectionMode int

const (
	withReflection reflectionMode = iota
	withoutReflection
	// withV1AlphaReflectionOnly serves only the older reflection protocol,
	// as grpc-dotnet servers do.
	withV1AlphaReflectionOnly
)

func startHealthServer(t *testing.T, mode reflectionMode) *healthServer {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	hs := &healthServer{endpoint: listener.Addr().String()}

	server := grpc.NewServer(grpc.UnaryInterceptor(func(
		ctx context.Context, req any, _ *grpc.UnaryServerInfo, handler grpc.UnaryHandler,
	) (any, error) {
		md, _ := metadata.FromIncomingContext(ctx)
		hs.mu.Lock()
		hs.metadata = md
		hs.mu.Unlock()
		return handler(ctx, req)
	}))
	checker := health.NewServer()
	checker.SetServingStatus(servingService, healthpb.HealthCheckResponse_SERVING)
	checker.SetServingStatus(defaultStatusService, healthpb.HealthCheckResponse_UNKNOWN)
	healthpb.RegisterHealthServer(server, checker)
	switch mode {
	case withReflection:
		reflection.Register(server)
	case withV1AlphaReflectionOnly:
		rpbalpha.RegisterServerReflectionServer(server, reflection.NewServer(reflection.ServerOptions{Services: server}))
	}

	go func() { _ = server.Serve(listener) }()
	t.Cleanup(server.Stop)
	return hs
}

func (hs *healthServer) received(key string) []string {
	hs.mu.Lock()
	defer hs.mu.Unlock()
	return hs.metadata.Get(key)
}

func healthItem(endpoint string, events ...postman.Event) *postman.Item {
	return &postman.Item{
		Name: "Check",
		Request: &postman.Request{
			Method: requestMethod,
			Header: []postman.Header{{Key: "{{" + callerKeyVar + "}}", Value: "{{" + callerValueVar + "}}"}},
			Body:   &postman.Body{Mode: "raw", Raw: `{"service": "{{` + serviceVariable + `}}"}`},
			URL:    postman.URL{Raw: "grpc://" + endpoint + healthCheckPath},
		},
		Events: events,
	}
}

func scopeOf(item *postman.Item, collection *postman.Collection, environment *postman.Environment) *script.RequestScope {
	return &script.RequestScope{Collection: collection, Environment: environment, Item: item}
}

func scriptEvent(listen string, lines ...string) postman.Event {
	return postman.Event{Listen: listen, Script: postman.Script{Type: "text/javascript", Exec: lines}}
}

func testEnvironment(service string) *postman.Environment {
	return &postman.Environment{Values: []postman.EnvVariable{
		{Key: callerKeyVar, Value: callerKey, Enabled: true},
		{Key: callerValueVar, Value: base64.RawStdEncoding.EncodeToString([]byte(callerValue)), Enabled: true},
		{Key: serviceVariable, Value: service, Enabled: true},
	}}
}

func envValue(env *postman.Environment, key string) string {
	for _, v := range env.Values {
		if v.Key == key {
			return v.Value
		}
	}
	return ""
}

func TestExecutor_Execute_SendsMetadataAndRunsScripts(t *testing.T) {
	server := startHealthServer(t, withReflection)
	collection := &postman.Collection{}
	environment := testEnvironment(unknownService)
	item := healthItem(server.endpoint,
		scriptEvent("prerequest", "pm.environment.set('"+serviceVariable+"', '"+servingService+"');"),
		scriptEvent("test",
			"pm.environment.set('"+statusVariable+"', pm.response.json().status);",
			"pm.test('is ok', function () { pm.response.to.have.status(200); });",
		),
	)

	resp, result := NewExecutor().ExecuteContext(context.Background(), item.Request, scopeOf(item, collection, environment), nil)

	if resp.Error != nil {
		t.Fatalf("unexpected error: %v", resp.Error)
	}
	if resp.StatusCode != okStatusCode || resp.Status != "OK" {
		t.Errorf("status = %d %q, want %d OK", resp.StatusCode, resp.Status, okStatusCode)
	}
	if got := envValue(environment, statusVariable); got != "SERVING" {
		t.Errorf("%s = %q, want SERVING (test script should read the response)", statusVariable, got)
	}
	if result == nil || len(result.Tests) != 1 || !result.Tests[0].Passed {
		t.Errorf("tests = %+v, want one passing test", result)
	}
	if got := server.received(callerKey); len(got) != 1 || got[0] != callerValue {
		t.Errorf("metadata %s = %v, want [%q] (base64 decoded once)", callerKey, got, callerValue)
	}
	if resp.RequestHeaders[callerKey] == "" {
		t.Errorf("request headers %v should record the resolved metadata", resp.RequestHeaders)
	}
}

func TestExecutor_Execute_ReportsGRPCStatusOnFailure(t *testing.T) {
	server := startHealthServer(t, withReflection)
	environment := testEnvironment(unknownService)
	item := healthItem(server.endpoint, scriptEvent("test", "pm.environment.set('"+statusVariable+"', 'ran');"))

	resp, _ := NewExecutor().ExecuteContext(context.Background(), item.Request, scopeOf(item, &postman.Collection{}, environment), nil)

	if resp.Error == nil {
		t.Fatal("expected an error for an unknown health service")
	}
	if resp.Status != expectedNotFound {
		t.Errorf("status = %q, want %q", resp.Status, expectedNotFound)
	}
	if envValue(environment, statusVariable) != "" {
		t.Error("test scripts should not run when the call fails")
	}
}

func TestExecutor_Execute_UsesV1AlphaReflectionWhenV1IsMissing(t *testing.T) {
	server := startHealthServer(t, withV1AlphaReflectionOnly)
	item := healthItem(server.endpoint)

	resp, _ := NewExecutor().ExecuteContext(context.Background(), item.Request, scopeOf(item, &postman.Collection{}, testEnvironment(servingService)), nil)

	if resp.Error != nil {
		t.Fatalf("expected the call to succeed over v1alpha reflection, got %v", resp.Error)
	}
}

func TestClient_ListServices_UsesV1AlphaReflectionWhenV1IsMissing(t *testing.T) {
	server := startHealthServer(t, withV1AlphaReflectionOnly)
	client, err := NewClient(server.endpoint, false)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()

	services, err := client.ListServices(context.Background())

	if err != nil || len(services) != 1 || services[0].Name != "grpc.health.v1.Health" {
		t.Errorf("expected the health service over v1alpha reflection, got %+v (%v)", services, err)
	}
}

func TestExecutor_Execute_StopsOnPreRequestScriptError(t *testing.T) {
	server := startHealthServer(t, withReflection)
	item := healthItem(server.endpoint, scriptEvent("prerequest", "throw new Error('boom');"))

	resp, _ := NewExecutor().ExecuteContext(context.Background(), item.Request, scopeOf(item, &postman.Collection{}, testEnvironment(servingService)), nil)

	if resp.Error == nil {
		t.Fatal("expected the pre-request script error")
	}
	if got := server.received(callerKey); got != nil {
		t.Errorf("request should not be sent, server saw metadata %v", got)
	}
}

func TestExecutor_Execute_ResolvesVariablesWithoutItem(t *testing.T) {
	server := startHealthServer(t, withReflection)
	environment := testEnvironment(servingService)
	item := healthItem(server.endpoint)
	variables := postman.GetAllVariables(nil, nil, environment)

	resp, result := NewExecutor().ExecuteContext(context.Background(), item.Request, nil, variables)

	if resp.Error != nil {
		t.Fatalf("unexpected error: %v", resp.Error)
	}
	if result != nil {
		t.Errorf("no item means no scripts, got %+v", result)
	}
	if resp.Body == "" {
		t.Error("expected a JSON response body")
	}
}

// writeHealthProtoset writes the grpc.health.v1 descriptors the way
// protoc --include_imports --descriptor_set_out would.
func writeHealthProtoset(t *testing.T) string {
	t.Helper()
	set := &descriptorpb.FileDescriptorSet{File: []*descriptorpb.FileDescriptorProto{
		protodesc.ToFileDescriptorProto(healthpb.File_grpc_health_v1_health_proto),
	}}
	data, err := proto.Marshal(set)
	if err != nil {
		t.Fatalf("marshal descriptor set: %v", err)
	}
	path := filepath.Join(t.TempDir(), "health.protoset")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatalf("write descriptor set: %v", err)
	}
	return path
}

func TestExecutor_Execute_FallsBackToProtosetWithoutReflection(t *testing.T) {
	server := startHealthServer(t, withoutReflection)
	environment := testEnvironment(servingService)
	environment.Values = append(environment.Values, postman.EnvVariable{Key: ProtosetVariable, Value: writeHealthProtoset(t), Enabled: true})
	item := healthItem(server.endpoint)

	resp, _ := NewExecutor().ExecuteContext(context.Background(), item.Request, scopeOf(item, &postman.Collection{}, environment), nil)

	if resp.Error != nil {
		t.Fatalf("unexpected error: %v", resp.Error)
	}
	if resp.Body != `{"status":"SERVING"}` {
		t.Errorf("body = %s, want the SERVING status", resp.Body)
	}
}

func TestExecutor_Execute_ReportsMissingReflectionWithoutProtoset(t *testing.T) {
	server := startHealthServer(t, withoutReflection)
	item := healthItem(server.endpoint)

	resp, _ := NewExecutor().ExecuteContext(context.Background(), item.Request, scopeOf(item, &postman.Collection{}, testEnvironment(servingService)), nil)

	if !errors.Is(resp.Error, ErrReflectionUnavailable) {
		t.Errorf("error = %v, want ErrReflectionUnavailable", resp.Error)
	}
}

func TestLoadProtoset_UnreadableFile(t *testing.T) {
	if _, err := loadProtoset(filepath.Join(t.TempDir(), "missing.protoset")); err == nil {
		t.Error("expected an error for a missing descriptor set")
	}
}

func TestBuildMetadata(t *testing.T) {
	encoded := base64.StdEncoding.EncodeToString([]byte(callerValue))
	md := buildMetadata([]postman.Header{
		{Key: "X-Caller-Info-Bin", Value: encoded},
		{Key: "x-raw-bin", Value: "not base64!"},
		{Key: "x-tenant", Value: encoded},
		{Key: "", Value: "ignored"},
	})

	cases := map[string]string{
		callerKey:   callerValue,
		"x-raw-bin": "not base64!",
		"x-tenant":  encoded,
	}
	for key, want := range cases {
		if got := md.Get(key); len(got) != 1 || got[0] != want {
			t.Errorf("%s = %v, want [%q]", key, got, want)
		}
	}
	if md.Len() != len(cases) {
		t.Errorf("metadata has %d keys, want %d (empty keys are skipped)", md.Len(), len(cases))
	}
}
