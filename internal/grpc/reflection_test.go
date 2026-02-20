package grpc

import (
	"testing"
)

func TestNewClient_BadEndpoint(t *testing.T) {
	// NewClient with grpc.NewClient does not dial immediately, so it succeeds
	// even for unreachable endpoints. Verify we get a non-nil client back.
	client, err := NewClient("localhost:1")
	if err != nil {
		t.Fatalf("NewClient returned unexpected error: %v", err)
	}
	if client == nil {
		t.Fatal("expected non-nil client")
	}
	_ = client.Close()
}

func TestNewClient_InvalidAddress(t *testing.T) {
	// An address that cannot even be parsed as a gRPC target should still
	// produce a client (grpc.NewClient is lazy) — confirm no panic.
	client, err := NewClient("///invalid:::addr")
	// May or may not error depending on the resolver; just ensure no panic.
	if err == nil && client != nil {
		_ = client.Close()
	}
}
