package tui

// Positions of the gRPC edit form's fields in grpcEditFieldNames.
const (
	grpcFieldTLS            = 5
	grpcFieldInjectDefaults = 6
)

var grpcEditFieldNames = []string{"Name", "Endpoint", "Service/Method", "Metadata", "Message", "TLS", "Inject defaults"}

const (
	onLabel  = "On"
	offLabel = "Off"
)

func onOffLabel(on bool) string {
	if on {
		return onLabel
	}
	return offLabel
}
