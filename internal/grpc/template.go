package grpc

import (
	"encoding/json"
	"strings"

	"google.golang.org/protobuf/types/descriptorpb"
)

const maxTemplateDepth = 5

type descriptorSet struct {
	messages map[string]*descriptorpb.DescriptorProto
}

func buildDescriptorSet(files []*descriptorpb.FileDescriptorProto) descriptorSet {
	ds := descriptorSet{messages: make(map[string]*descriptorpb.DescriptorProto)}
	for _, fd := range files {
		pkg := fd.GetPackage()
		indexMessages(ds, pkg, fd.GetMessageType())
	}
	return ds
}

func indexMessages(ds descriptorSet, pkg string, messages []*descriptorpb.DescriptorProto) {
	for _, msg := range messages {
		var fqn string
		if pkg != "" {
			fqn = "." + pkg + "." + msg.GetName()
		} else {
			fqn = "." + msg.GetName()
		}
		ds.messages[fqn] = msg
		nestedPkg := fqn[1:] // strip leading dot for nested package prefix
		indexMessages(ds, nestedPkg, msg.GetNestedType())
	}
}

// BuildMessageTemplate returns a pretty-printed JSON template for the given proto message type.
// typeName must be the fully-qualified name with leading dot, e.g. ".helloworld.HelloRequest".
func BuildMessageTemplate(files []*descriptorpb.FileDescriptorProto, typeName string) string {
	ds := buildDescriptorSet(files)
	value := buildTemplateValue(ds, typeName, 0)
	bytes, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return "{}"
	}
	return string(bytes)
}

func buildTemplateValue(ds descriptorSet, typeName string, depth int) any {
	if depth >= maxTemplateDepth {
		return map[string]any{}
	}

	msg, ok := ds.messages[typeName]
	if !ok {
		return map[string]any{}
	}

	result := map[string]any{}
	for _, field := range msg.GetField() {
		jsonName := field.GetJsonName()
		if jsonName == "" {
			jsonName = field.GetName()
		}

		isRepeated := field.GetLabel() == descriptorpb.FieldDescriptorProto_LABEL_REPEATED

		if isMapEntry(msg, field) {
			result[jsonName] = map[string]any{}
			continue
		}

		if isRepeated {
			result[jsonName] = []any{}
			continue
		}

		result[jsonName] = buildScalarValue(ds, field, depth)
	}
	return result
}

func isMapEntry(msg *descriptorpb.DescriptorProto, field *descriptorpb.FieldDescriptorProto) bool {
	if field.GetType() != descriptorpb.FieldDescriptorProto_TYPE_MESSAGE {
		return false
	}
	if field.GetLabel() != descriptorpb.FieldDescriptorProto_LABEL_REPEATED {
		return false
	}
	typeName := field.GetTypeName()
	// Find the nested type descriptor to check if it's a map entry
	for _, nested := range msg.GetNestedType() {
		nestedShortName := "." + nested.GetName()
		if strings.HasSuffix(typeName, nestedShortName) && nested.GetOptions().GetMapEntry() {
			return true
		}
	}
	return false
}

func buildScalarValue(ds descriptorSet, field *descriptorpb.FieldDescriptorProto, depth int) interface{} {
	switch field.GetType() {
	case descriptorpb.FieldDescriptorProto_TYPE_MESSAGE:
		return buildTemplateValue(ds, field.GetTypeName(), depth+1)
	case descriptorpb.FieldDescriptorProto_TYPE_ENUM:
		return 0
	case descriptorpb.FieldDescriptorProto_TYPE_BOOL:
		return false
	case descriptorpb.FieldDescriptorProto_TYPE_STRING:
		return ""
	case descriptorpb.FieldDescriptorProto_TYPE_BYTES:
		return ""
	case descriptorpb.FieldDescriptorProto_TYPE_DOUBLE,
		descriptorpb.FieldDescriptorProto_TYPE_FLOAT:
		return 0.0
	default:
		// All integer types
		return 0
	}
}
