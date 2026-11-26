package grpc

import (
	"errors"
	"io"
	"testing"

	"google.golang.org/grpc/codes"
	rpb "google.golang.org/grpc/reflection/grpc_reflection_v1"
	"google.golang.org/grpc/status"
)

func TestNewClient_InsecureBadEndpoint(t *testing.T) {
	// NewClient with grpc.NewClient does not dial immediately, so it succeeds
	// even for unreachable endpoints. Verify we get a non-nil client back.
	client, err := NewClient("localhost:1", false)
	if err != nil {
		t.Fatalf("NewClient returned unexpected error: %v", err)
	}
	if client == nil {
		t.Fatal("expected non-nil client")
	}
	_ = client.Close()
}

func TestNewClient_TLSBadEndpoint(t *testing.T) {
	client, err := NewClient("localhost:1", true)
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
	client, err := NewClient("///invalid:::addr", false)
	// May or may not error depending on the resolver; just ensure no panic.
	if err == nil && client != nil {
		_ = client.Close()
	}
}

// endedReflectionStream is a reflection stream the server has already ended:
// Send reports a bare io.EOF and Recv the status the server ended it with.
type endedReflectionStream struct {
	reflectionStream
	sendErr error
	ended   error
}

func (s endedReflectionStream) Send(*rpb.ServerReflectionRequest) error { return s.sendErr }

func (s endedReflectionStream) Recv() (*rpb.ServerReflectionResponse, error) { return nil, s.ended }

func TestRoundTrip_SendEOFReportsTheStatusTheServerEndedWith(t *testing.T) {
	stream := endedReflectionStream{sendErr: io.EOF, ended: status.Error(codes.Unimplemented, "unknown service grpc.reflection.v1.ServerReflection")}

	_, err := roundTrip(stream, &rpb.ServerReflectionRequest{})

	if status.Code(err) != codes.Unimplemented {
		t.Fatalf("error = %v, want the Unimplemented status so the v1alpha fallback runs", err)
	}
}

func TestRoundTrip_SendFailureOtherThanEOFIsReturned(t *testing.T) {
	sendErr := errors.New("connection refused")
	stream := endedReflectionStream{sendErr: sendErr, ended: status.Error(codes.Unimplemented, "unused")}

	_, err := roundTrip(stream, &rpb.ServerReflectionRequest{})

	if !errors.Is(err, sendErr) {
		t.Fatalf("error = %v, want the send failure", err)
	}
}

func TestGetFileDescriptorsForSymbol_ServerEndedV1StreamFallsBackAsUnimplemented(t *testing.T) {
	stream := endedReflectionStream{sendErr: io.EOF, ended: status.Error(codes.Unimplemented, "unknown service grpc.reflection.v1.ServerReflection")}

	_, err := getFileDescriptorsForSymbol(stream, "develophub.services.insurify.v2.HomeInsurancePolicyService")

	if status.Code(err) != codes.Unimplemented {
		t.Fatalf("error = %v, want Unimplemented so withReflectionStream retries over v1alpha", err)
	}
}
