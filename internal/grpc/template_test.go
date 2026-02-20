package grpc

import (
	"encoding/json"
	"testing"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/descriptorpb"
)

// fileWithMessage builds a minimal FileDescriptorProto containing the given messages.
func fileWithMessage(pkg string, messages ...*descriptorpb.DescriptorProto) *descriptorpb.FileDescriptorProto {
	return &descriptorpb.FileDescriptorProto{
		Name:        proto.String("test.proto"),
		Package:     proto.String(pkg),
		MessageType: messages,
	}
}

func fieldOf(name string, number int32, typ descriptorpb.FieldDescriptorProto_Type, label descriptorpb.FieldDescriptorProto_Label) *descriptorpb.FieldDescriptorProto {
	return &descriptorpb.FieldDescriptorProto{
		Name:     proto.String(name),
		Number:   proto.Int32(number),
		Type:     typ.Enum(),
		Label:    label.Enum(),
		JsonName: proto.String(name),
	}
}

func messageFieldOf(name string, number int32, typeName string, label descriptorpb.FieldDescriptorProto_Label) *descriptorpb.FieldDescriptorProto {
	return &descriptorpb.FieldDescriptorProto{
		Name:     proto.String(name),
		Number:   proto.Int32(number),
		Type:     descriptorpb.FieldDescriptorProto_TYPE_MESSAGE.Enum(),
		TypeName: proto.String(typeName),
		Label:    label.Enum(),
		JsonName: proto.String(name),
	}
}

func TestBuildMessageTemplate_ScalarFields(t *testing.T) {
	msg := &descriptorpb.DescriptorProto{
		Name: proto.String("Request"),
		Field: []*descriptorpb.FieldDescriptorProto{
			fieldOf("name", 1, descriptorpb.FieldDescriptorProto_TYPE_STRING, descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL),
			fieldOf("count", 2, descriptorpb.FieldDescriptorProto_TYPE_INT32, descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL),
			fieldOf("active", 3, descriptorpb.FieldDescriptorProto_TYPE_BOOL, descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL),
			fieldOf("ratio", 4, descriptorpb.FieldDescriptorProto_TYPE_DOUBLE, descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL),
		},
	}
	fd := fileWithMessage("example", msg)
	result := BuildMessageTemplate([]*descriptorpb.FileDescriptorProto{fd}, ".example.Request")

	var parsed map[string]any
	if err := json.Unmarshal([]byte(result), &parsed); err != nil {
		t.Fatalf("result is not valid JSON: %v\nraw: %s", err, result)
	}

	if parsed["name"] != "" {
		t.Errorf("expected empty string for name, got %v", parsed["name"])
	}
	if parsed["active"] != false {
		t.Errorf("expected false for active, got %v", parsed["active"])
	}
	if _, ok := parsed["count"]; !ok {
		t.Error("expected count field in template")
	}
}

func TestBuildMessageTemplate_RepeatedField(t *testing.T) {
	msg := &descriptorpb.DescriptorProto{
		Name: proto.String("Request"),
		Field: []*descriptorpb.FieldDescriptorProto{
			fieldOf("tags", 1, descriptorpb.FieldDescriptorProto_TYPE_STRING, descriptorpb.FieldDescriptorProto_LABEL_REPEATED),
		},
	}
	fd := fileWithMessage("example", msg)
	result := BuildMessageTemplate([]*descriptorpb.FileDescriptorProto{fd}, ".example.Request")

	var parsed map[string]any
	if err := json.Unmarshal([]byte(result), &parsed); err != nil {
		t.Fatalf("result is not valid JSON: %v", err)
	}

	tags, ok := parsed["tags"].([]any)
	if !ok {
		t.Fatalf("expected tags to be an array, got %T: %v", parsed["tags"], parsed["tags"])
	}
	if len(tags) != 0 {
		t.Errorf("expected empty array, got %v", tags)
	}
}

func TestBuildMessageTemplate_NestedMessage(t *testing.T) {
	inner := &descriptorpb.DescriptorProto{
		Name: proto.String("Inner"),
		Field: []*descriptorpb.FieldDescriptorProto{
			fieldOf("value", 1, descriptorpb.FieldDescriptorProto_TYPE_STRING, descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL),
		},
	}
	outer := &descriptorpb.DescriptorProto{
		Name: proto.String("Outer"),
		Field: []*descriptorpb.FieldDescriptorProto{
			messageFieldOf("inner", 1, ".example.Inner", descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL),
		},
	}
	fd := fileWithMessage("example", inner, outer)
	result := BuildMessageTemplate([]*descriptorpb.FileDescriptorProto{fd}, ".example.Outer")

	var parsed map[string]any
	if err := json.Unmarshal([]byte(result), &parsed); err != nil {
		t.Fatalf("result is not valid JSON: %v", err)
	}

	innerVal, ok := parsed["inner"].(map[string]any)
	if !ok {
		t.Fatalf("expected inner to be an object, got %T: %v", parsed["inner"], parsed["inner"])
	}
	if innerVal["value"] != "" {
		t.Errorf("expected empty string for inner.value, got %v", innerVal["value"])
	}
}

func TestBuildMessageTemplate_UnknownType(t *testing.T) {
	result := BuildMessageTemplate(nil, ".nonexistent.Type")
	if result != "{}" {
		t.Errorf("expected {} for unknown type, got %q", result)
	}
}

func TestBuildMessageTemplate_MaxDepth(t *testing.T) {
	// Self-referential message should not cause infinite recursion.
	selfRef := &descriptorpb.DescriptorProto{
		Name: proto.String("Node"),
		Field: []*descriptorpb.FieldDescriptorProto{
			messageFieldOf("child", 1, ".example.Node", descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL),
		},
	}
	fd := fileWithMessage("example", selfRef)
	result := BuildMessageTemplate([]*descriptorpb.FileDescriptorProto{fd}, ".example.Node")

	var parsed map[string]any
	if err := json.Unmarshal([]byte(result), &parsed); err != nil {
		t.Fatalf("result is not valid JSON: %v", err)
	}
}
