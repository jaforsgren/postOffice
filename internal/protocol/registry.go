package protocol

import (
	"fmt"
	"postOffice/internal/amqp"
	"postOffice/internal/grpc"
	"postOffice/internal/history"
	"postOffice/internal/http"
	"postOffice/internal/postman"
	"postOffice/internal/servicebus"
	"postOffice/internal/shell"
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
// of any protocol send pm.sendRequest calls with httpExecutor. Bash scripts
// run on terminal, so they can prompt; a nil terminal runs them without input.
func NewBuiltinRegistry(httpExecutor *http.Executor, terminal shell.Terminal) *Registry {
	return NewRegistry(
		// The BASH method marks a script whatever its URL, so it claims first.
		newRespondingProtocol(history.ProtocolBash, (*postman.Item).IsBash, postman.MethodBash, shell.NewExecutor(terminal).ExecuteContext, httpExecutor.SendRequest),
		// Peeks are asb:// requests too, so they must claim before Service Bus sends.
		newRespondingProtocol(history.ProtocolServiceBusPeek, (*postman.Item).IsServiceBusPeek, postman.MethodServiceBusPeek, servicebus.PeekContext, httpExecutor.SendRequest),
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
