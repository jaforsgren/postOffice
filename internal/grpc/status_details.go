package grpc

import (
	"fmt"
	"strings"

	"google.golang.org/genproto/googleapis/rpc/errdetails"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
)

const (
	statusDetailsHeading = "Details:"
	statusDetailIndent   = "  - "
)

// callError reports a failed call together with the details the server
// attached to its status (field violations, error info, ...), which the
// status message alone leaves out.
func callError(fullMethod string, err error) error {
	details := statusDetailLines(err)
	if len(details) == 0 {
		return fmt.Errorf("gRPC call %q failed: %w", fullMethod, err)
	}
	return fmt.Errorf("gRPC call %q failed: %w\n%s\n%s", fullMethod, err, statusDetailsHeading, strings.Join(details, "\n"))
}

// statusDetailLines renders each status detail on its own line. Importing
// errdetails registers the google.rpc detail types, so they decode instead
// of arriving as raw bytes.
func statusDetailLines(err error) []string {
	st, ok := status.FromError(err)
	if !ok {
		return nil
	}

	var lines []string
	for i, detail := range st.Details() {
		switch typed := detail.(type) {
		case *errdetails.BadRequest:
			for _, violation := range typed.GetFieldViolations() {
				lines = append(lines, statusDetailIndent+violation.GetField()+": "+violation.GetDescription())
			}
		case *errdetails.PreconditionFailure:
			for _, violation := range typed.GetViolations() {
				lines = append(lines, statusDetailIndent+violation.GetType()+" "+violation.GetSubject()+": "+violation.GetDescription())
			}
		case proto.Message:
			lines = append(lines, statusDetailIndent+string(typed.ProtoReflect().Descriptor().FullName())+": "+compactJSON(typed))
		default:
			lines = append(lines, statusDetailIndent+st.Proto().GetDetails()[i].GetTypeUrl()+" (unknown detail type)")
		}
	}
	return lines
}

func compactJSON(message proto.Message) string {
	out, err := protojson.MarshalOptions{Multiline: false}.Marshal(message)
	if err != nil {
		return err.Error()
	}
	return string(out)
}
