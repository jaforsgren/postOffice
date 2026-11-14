package servicebus

import (
	"context"
	"encoding/json"
	"fmt"
	"postOffice/internal/http"
	"postOffice/internal/postman"
	"strconv"
	"strings"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/messaging/azservicebus"
)

// Peek request headers. Like Connection-String, they configure the peek and
// are not message properties.
const (
	headerMaxMessages  = "Max-Messages"
	headerDeadLetter   = "Dead-Letter"
	headerPollInterval = "Poll-Interval"

	// DefaultMaxMessages is how many messages one peek returns without Max-Messages.
	DefaultMaxMessages = 50
	// DefaultPollInterval is how often a watcher peeks without Poll-Interval.
	DefaultPollInterval = time.Second
	// minPollInterval keeps a watcher from hammering the broker.
	minPollInterval = 100 * time.Millisecond

	peekStatusCode = 200
	deadLetterTag  = " (dead-letter)"
)

// peekRequest is a PEEK request with its variables resolved and headers parsed.
type peekRequest struct {
	target           PeekTarget
	connectionString string
	maxMessages      int
	pollInterval     time.Duration
	url              string
	headers          map[string]string
}

// String names the target as "topic/subscription" or "queue", tagging a
// dead-letter queue.
func (t PeekTarget) String() string {
	name := t.Entity
	if t.Subscription != "" {
		name += "/" + t.Subscription
	}
	if t.DeadLetter {
		name += deadLetterTag
	}
	return name
}

// ParsePeekTarget reads the queue or topic subscription from a resolved PEEK
// URL: asb://queue or asb://topic/subscription.
func ParsePeekTarget(rawURL string) (PeekTarget, error) {
	path, ok := entityPath(rawURL)
	if !ok || path == "" {
		return PeekTarget{}, invalidPeekTarget(rawURL)
	}
	parts := strings.Split(path, "/")
	switch {
	case len(parts) == 1:
		return PeekTarget{Entity: parts[0]}, nil
	case len(parts) == 2 && parts[0] != "" && parts[1] != "":
		return PeekTarget{Entity: parts[0], Subscription: parts[1]}, nil
	default:
		return PeekTarget{}, invalidPeekTarget(rawURL)
	}
}

func invalidPeekTarget(rawURL string) error {
	return fmt.Errorf("%w: %q, want %s<queue> or %s<topic>/<subscription>", ErrInvalidPeekTarget, rawURL, urlScheme, urlScheme)
}

func resolvePeekRequest(req *postman.Request, variables []postman.VariableSource) (peekRequest, error) {
	request := peekRequest{
		url:          postman.ResolveVariables(req.URL.Raw, variables),
		headers:      resolveHeaders(req, variables),
		maxMessages:  DefaultMaxMessages,
		pollInterval: DefaultPollInterval,
	}
	target, err := ParsePeekTarget(request.url)
	if err != nil {
		return request, err
	}
	request.target = target

	if request.connectionString = request.headers[headerConnectionString]; request.connectionString == "" {
		return request, fmt.Errorf("missing required header: %s", headerConnectionString)
	}
	if value := request.headers[headerMaxMessages]; value != "" {
		if request.maxMessages, err = strconv.Atoi(value); err != nil || request.maxMessages < 1 {
			return request, fmt.Errorf("%w: %s must be a positive number, got %q", ErrInvalidPeekHeader, headerMaxMessages, value)
		}
	}
	if value := request.headers[headerDeadLetter]; value != "" {
		if request.target.DeadLetter, err = strconv.ParseBool(value); err != nil {
			return request, fmt.Errorf("%w: %s must be true or false, got %q", ErrInvalidPeekHeader, headerDeadLetter, value)
		}
	}
	if value := request.headers[headerPollInterval]; value != "" {
		if request.pollInterval, err = time.ParseDuration(value); err != nil || request.pollInterval < minPollInterval {
			return request, fmt.Errorf("%w: %s must be a duration of at least %v, got %q", ErrInvalidPeekHeader, headerPollInterval, minPollInterval, value)
		}
	}
	return request, nil
}

// Watcher peeks a queue or subscription repeatedly, each time returning only
// the messages that arrived since the previous peek. A Watcher is not safe
// for concurrent use.
type Watcher struct {
	request  peekRequest
	client   *azservicebus.Client
	receiver *azservicebus.Receiver
}

// OpenWatcher resolves req against variables and prepares a peek of its
// target. It doesn't connect: the first Poll does.
func OpenWatcher(req *postman.Request, variables []postman.VariableSource) (*Watcher, error) {
	request, err := resolvePeekRequest(req, variables)
	if err != nil {
		return nil, err
	}
	return openWatcher(request)
}

func openWatcher(request peekRequest) (*Watcher, error) {
	client, err := azservicebus.NewClientFromConnectionString(request.connectionString, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create Service Bus client: %w", err)
	}
	options := &azservicebus.ReceiverOptions{}
	if request.target.DeadLetter {
		options.SubQueue = azservicebus.SubQueueDeadLetter
	}
	var receiver *azservicebus.Receiver
	if request.target.Subscription == "" {
		receiver, err = client.NewReceiverForQueue(request.target.Entity, options)
	} else {
		receiver, err = client.NewReceiverForSubscription(request.target.Entity, request.target.Subscription, options)
	}
	if err != nil {
		_ = client.Close(context.Background())
		return nil, fmt.Errorf("failed to create receiver for %q: %w", request.target, err)
	}
	return &Watcher{request: request, client: client, receiver: receiver}, nil
}

// Target is the queue or subscription the watcher peeks.
func (w *Watcher) Target() PeekTarget {
	return w.request.target
}

// PollInterval is how long to wait between polls.
func (w *Watcher) PollInterval() time.Duration {
	return w.request.pollInterval
}

// Poll returns every message that arrived since the previous poll, oldest
// first. The first poll returns the messages already waiting.
func (w *Watcher) Poll(ctx context.Context) ([]PeekedMessage, error) {
	var messages []PeekedMessage
	for {
		page, err := w.peekPage(ctx)
		messages = append(messages, page...)
		if err != nil || len(page) < w.request.maxMessages {
			return messages, err
		}
	}
}

// peekPage peeks up to Max-Messages messages after the last one peeked.
func (w *Watcher) peekPage(ctx context.Context) ([]PeekedMessage, error) {
	received, err := w.receiver.PeekMessages(ctx, w.request.maxMessages, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to peek %q: %w", w.request.target, err)
	}
	messages := make([]PeekedMessage, len(received))
	for i, message := range received {
		messages[i] = newPeekedMessage(message)
	}
	return messages, nil
}

// Close releases the watcher's connection.
func (w *Watcher) Close() {
	_ = w.receiver.Close(context.Background())
	_ = w.client.Close(context.Background())
}

// PeekContext peeks up to Max-Messages messages waiting in the queue or
// subscription described by req, without locking or removing them, and
// gives up when ctx is cancelled.
//
// URL format: asb://queue or asb://topic/subscription
// Headers:
//   - Connection-String: Azure Service Bus connection string (required)
//   - Max-Messages: how many messages to return (default DefaultMaxMessages)
//   - Dead-Letter: true peeks the dead-letter queue
//
// The response body is a JSON array of PeekedMessage, oldest first.
func PeekContext(ctx context.Context, req *postman.Request, variables []postman.VariableSource) *http.Response {
	start := time.Now()
	resp := &http.Response{RequestMethod: postman.MethodServiceBusPeek}
	request, err := resolvePeekRequest(req, variables)
	resp.RequestURL = request.url
	resp.RequestHeaders = request.headers
	if err == nil {
		err = peekInto(ctx, request, resp)
	}
	resp.Error = err
	resp.Duration = time.Since(start)
	return resp
}

func peekInto(ctx context.Context, request peekRequest, resp *http.Response) error {
	watcher, err := openWatcher(request)
	if err != nil {
		return err
	}
	defer watcher.Close()

	peekCtx, cancel := context.WithTimeout(ctx, defaultTimeout)
	defer cancel()
	messages, err := watcher.peekPage(peekCtx)
	if err != nil {
		return err
	}
	body, err := json.MarshalIndent(nonNil(messages), "", "  ")
	if err != nil {
		return fmt.Errorf("encode peeked messages: %w", err)
	}
	resp.StatusCode = peekStatusCode
	resp.Status = fmt.Sprintf("%d message(s) peeked", len(messages))
	resp.Body = string(body)
	return nil
}

// nonNil makes an empty peek encode as [] rather than null.
func nonNil(messages []PeekedMessage) []PeekedMessage {
	if messages == nil {
		return []PeekedMessage{}
	}
	return messages
}

func newPeekedMessage(message *azservicebus.ReceivedMessage) PeekedMessage {
	peeked := PeekedMessage{
		MessageID:             message.MessageID,
		Subject:               valueOf(message.Subject),
		ContentType:           valueOf(message.ContentType),
		CorrelationID:         valueOf(message.CorrelationID),
		ApplicationProperties: message.ApplicationProperties,
		Body:                  jsonBody(message.Body),
	}
	if message.SequenceNumber != nil {
		peeked.SequenceNumber = *message.SequenceNumber
	}
	if message.EnqueuedTime != nil {
		peeked.EnqueuedTime = *message.EnqueuedTime
	}
	return peeked
}

func jsonBody(body []byte) json.RawMessage {
	if json.Valid(body) {
		return json.RawMessage(body)
	}
	// Marshalling a string can't fail.
	encoded, _ := json.Marshal(string(body))
	return encoded
}

func valueOf(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}
