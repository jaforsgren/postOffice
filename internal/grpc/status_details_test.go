package grpc

import (
	"encoding/base64"
	"fmt"
	"strings"
	"testing"

	"google.golang.org/genproto/googleapis/rpc/errdetails"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/anypb"
)

const (
	validationMessage = "One or more validation errors occurred."
	testFullMethod    = "/persona.PersonaService/CreatePerson"
)

func statusWithDetails(t *testing.T, details ...*anypb.Any) error {
	t.Helper()
	st := status.New(codes.InvalidArgument, validationMessage).Proto()
	st.Details = details
	return status.ErrorProto(st)
}

func mustAny(t *testing.T, detail *errdetails.BadRequest) *anypb.Any {
	t.Helper()
	packed, err := anypb.New(detail)
	if err != nil {
		t.Fatal(err)
	}
	return packed
}

func TestCallError_ListsFieldViolations(t *testing.T) {
	err := statusWithDetails(t, mustAny(t, &errdetails.BadRequest{FieldViolations: []*errdetails.BadRequest_FieldViolation{
		{Field: "Email", Description: "Email is invalid"},
		{Field: "Ssn", Description: "Ssn is required"},
	}}))

	got := callError(testFullMethod, err).Error()

	for _, want := range []string{validationMessage, statusDetailsHeading, statusDetailIndent + "Email: Email is invalid", statusDetailIndent + "Ssn: Ssn is required"} {
		if !strings.Contains(got, want) {
			t.Errorf("expected %q in %q", want, got)
		}
	}
}

func TestCallError_KeepsTheStatusCode(t *testing.T) {
	err := statusWithDetails(t, mustAny(t, &errdetails.BadRequest{}))

	if code := status.Code(callError(testFullMethod, err)); code != codes.InvalidArgument {
		t.Errorf("expected InvalidArgument to survive wrapping, got %v", code)
	}
}

func TestCallError_WithoutDetailsIsUnchanged(t *testing.T) {
	err := status.Error(codes.NotFound, "no such person")

	got := callError(testFullMethod, err).Error()

	if strings.Contains(got, statusDetailsHeading) {
		t.Errorf("expected no details section, got %q", got)
	}
}

func TestStatusDetailLines_OtherKnownTypesAsJSON(t *testing.T) {
	packed, err := anypb.New(&errdetails.ErrorInfo{Reason: "PERSON_EXISTS", Domain: "persona"})
	if err != nil {
		t.Fatal(err)
	}

	lines := statusDetailLines(statusWithDetails(t, packed))

	if len(lines) != 1 || !strings.Contains(lines[0], "google.rpc.ErrorInfo") || !strings.Contains(lines[0], "PERSON_EXISTS") {
		t.Errorf("expected the error info as JSON, got %v", lines)
	}
}

func TestStatusDetailLines_UnknownTypeNamesIt(t *testing.T) {
	unknown := &anypb.Any{TypeUrl: "type.googleapis.com/acme.Custom", Value: []byte{1}}

	lines := statusDetailLines(statusWithDetails(t, unknown))

	if len(lines) != 1 || !strings.Contains(lines[0], "acme.Custom") {
		t.Errorf("expected the unknown type named, got %v", lines)
	}
}

func TestStatusDetailLines_NotAStatusError(t *testing.T) {
	if lines := statusDetailLines(fmt.Errorf("plain")); lines != nil {
		t.Errorf("expected no lines for a non-status error, got %v", lines)
	}
}

// personaBadRequest is the BadRequest detail the persona service returned
// for an invalid email, as raw bytes (grpcurl showed it base64 encoded).
const personaBadRequest = "ChkKBUVtYWlsEhBFbWFpbCBpcyBpbnZhbGlk"

func TestStatusDetailLines_DecodesThePersonaServicesBadRequest(t *testing.T) {
	value, err := base64.StdEncoding.DecodeString(personaBadRequest)
	if err != nil {
		t.Fatal(err)
	}
	packed := &anypb.Any{TypeUrl: "type.googleapis.com/google.rpc.BadRequest", Value: value}

	lines := statusDetailLines(statusWithDetails(t, packed))

	if len(lines) != 1 || lines[0] != statusDetailIndent+"Email: Email is invalid" {
		t.Errorf("expected the email violation, got %v", lines)
	}
}
