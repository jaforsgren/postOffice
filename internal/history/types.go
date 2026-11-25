package history

import "postOffice/internal/postman"

// Protocol identifies which executor a history entry must be replayed with.
type Protocol string

const (
	ProtocolHTTP       Protocol = "HTTP"
	ProtocolGRPC       Protocol = "gRPC"
	ProtocolServiceBus Protocol = "ASB"
	// ProtocolServiceBusPeek is a non-destructive read of a queue or subscription.
	ProtocolServiceBusPeek Protocol = "ASB-PEEK"
	ProtocolAMQP           Protocol = "AMQP"
	ProtocolBash           Protocol = "BASH"
)

// Entry is one recorded request execution. Snapshot holds the fully resolved
// request (method, URL/target, headers, body) and the response it produced.
type Entry struct {
	ItemID          string                `json:"item_id"`
	ItemName        string                `json:"item_name"`
	Protocol        Protocol              `json:"protocol"`
	EnvironmentName string                `json:"environment_name,omitempty"`
	Error           string                `json:"error,omitempty"`
	Snapshot        postman.SavedResponse `json:"snapshot"`
}
