package protocol

import (
	"fmt"
	"postOffice/internal/amqp"
	"postOffice/internal/grpc"
	"postOffice/internal/history"
	"postOffice/internal/http"
	"postOffice/internal/postman"
	"postOffice/internal/servicebus"
)

// Registry picks the Protocol that sends a request.
type Registry struct {
	protocols []Protocol
}

// NewRegistry registers protocols in claim order: an item is sent by the
// first protocol that claims it.
func NewRegistry(protocols ...Protocol) *Registry {
	return &Registry{protocols: protocols}
}

// NewBuiltinRegistry registers every protocol PostOffice ships with. Scripts
// of any protocol send pm.sendRequest calls with httpExecutor.
func NewBuiltinRegistry(httpExecutor *http.Executor) *Registry {
	return NewRegistry(
		newMessageProtocol(history.ProtocolServiceBus, (*postman.Item).IsServiceBus, servicebus.SendContext, httpExecutor.SendRequest),
		newMessageProtocol(history.ProtocolAMQP, (*postman.Item).IsAMQP, amqp.PublishContext, httpExecutor.SendRequest),
		newScriptedProtocol(history.ProtocolGRPC, (*postman.Item).IsGRPC, grpc.NewExecutor()),
		// HTTP claims every request, so it must stay last.
		newScriptedProtocol(history.ProtocolHTTP, (*postman.Item).IsRequest, httpExecutor),
	)
}

// For returns the protocol that sends item.
func (r *Registry) For(item *postman.Item) (Protocol, error) {
	for _, protocol := range r.protocols {
		if protocol.Claims(item) {
			return protocol, nil
		}
	}
	return nil, fmt.Errorf("%w: %q", ErrUnclaimedItem, item.Name)
}

// Named returns the protocol called name.
func (r *Registry) Named(name history.Protocol) (Protocol, error) {
	for _, protocol := range r.protocols {
		if protocol.Name() == name {
			return protocol, nil
		}
	}
	return nil, fmt.Errorf("%w: %q", ErrUnknownProtocol, name)
}
