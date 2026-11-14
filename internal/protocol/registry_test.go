package protocol

import (
	"errors"
	"postOffice/internal/history"
	"postOffice/internal/http"
	"postOffice/internal/postman"
	"testing"
)

func requestItem(method, rawURL string) *postman.Item {
	return &postman.Item{Name: method + " " + rawURL, Request: &postman.Request{Method: method, URL: postman.URL{Raw: rawURL}}}
}

func TestBuiltinRegistry_ForRoutesByProtocol(t *testing.T) {
	registry := NewBuiltinRegistry(http.NewExecutor())
	tests := []struct {
		name string
		item *postman.Item
		want history.Protocol
	}{
		{"HTTP", requestItem("GET", "https://example.com/users"), history.ProtocolHTTP},
		{"gRPC by method", requestItem("GRPC", "localhost:50051/svc.Greeter/SayHello"), history.ProtocolGRPC},
		{"gRPC by scheme", requestItem("POST", "grpcs://localhost:50051/svc.Greeter/SayHello"), history.ProtocolGRPC},
		{"Service Bus", requestItem("POST", "asb://orders"), history.ProtocolServiceBus},
		{"Service Bus peek", requestItem("PEEK", "asb://payments/postoffice-tap"), history.ProtocolServiceBusPeek},
		{"Service Bus peek, any case", requestItem("peek", "ASB://orders"), history.ProtocolServiceBusPeek},
		{"AMQP", requestItem("POST", "amqp://localhost:5672/orders"), history.ProtocolAMQP},
		{"AMQPS", requestItem("POST", "AMQPS://localhost:5671/orders"), history.ProtocolAMQP},
		{"Service Bus wins over a GRPC method", requestItem("GRPC", "asb://orders"), history.ProtocolServiceBus},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			protocol, err := registry.For(tt.item)
			if err != nil {
				t.Fatalf("For() error = %v", err)
			}
			if protocol.Name() != tt.want {
				t.Errorf("For() = %s, want %s", protocol.Name(), tt.want)
			}
		})
	}
}

func TestRegistry_ForRejectsFolders(t *testing.T) {
	folder := &postman.Item{Name: "Users", Items: []postman.Item{*requestItem("GET", "https://example.com")}}

	if _, err := NewBuiltinRegistry(http.NewExecutor()).For(folder); !errors.Is(err, ErrUnclaimedItem) {
		t.Errorf("For(folder) error = %v, want ErrUnclaimedItem", err)
	}
}

func TestRegistry_ForPicksFirstClaimingProtocol(t *testing.T) {
	claimsAll := (*postman.Item).IsRequest
	first := newScriptedProtocol("first", claimsAll, nil)
	second := newScriptedProtocol("second", claimsAll, nil)

	protocol, err := NewRegistry(first, second).For(requestItem("GET", "https://example.com"))
	if err != nil || protocol.Name() != "first" {
		t.Errorf("For() = %v, %v, want the first registered protocol", protocol, err)
	}
}

func TestBuiltinRegistry_NamedFindsEveryHistoryProtocol(t *testing.T) {
	registry := NewBuiltinRegistry(http.NewExecutor())
	for _, name := range []history.Protocol{history.ProtocolHTTP, history.ProtocolGRPC, history.ProtocolServiceBus, history.ProtocolServiceBusPeek, history.ProtocolAMQP} {
		protocol, err := registry.Named(name)
		if err != nil || protocol.Name() != name {
			t.Errorf("Named(%s) = %v, %v", name, protocol, err)
		}
	}
	if _, err := registry.Named("SMTP"); !errors.Is(err, ErrUnknownProtocol) {
		t.Errorf("Named(SMTP) error = %v, want ErrUnknownProtocol", err)
	}
}

func TestBuiltinRegistry_Labels(t *testing.T) {
	registry := NewBuiltinRegistry(http.NewExecutor())
	tests := []struct {
		item *postman.Item
		want string
	}{
		{requestItem("PATCH", "https://example.com"), "PATCH"},
		{requestItem("GRPC", "grpc://localhost:50051/svc.Greeter/SayHello"), "GRPC"},
		{requestItem("POST", "asb://orders"), "ASB"},
		{requestItem("peek", "asb://orders"), "PEEK"},
		{requestItem("POST", "amqp://localhost/orders"), "AMQP"},
	}
	for _, tt := range tests {
		protocol, err := registry.For(tt.item)
		if err != nil {
			t.Fatalf("For(%s) error = %v", tt.item.Name, err)
		}
		if got := protocol.Label(tt.item.Request); got != tt.want {
			t.Errorf("Label(%s) = %q, want %q", tt.item.Name, got, tt.want)
		}
	}
}
