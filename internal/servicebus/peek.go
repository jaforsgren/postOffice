package servicebus

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"maps"
	"postOffice/internal/http"
	"postOffice/internal/postman"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/messaging/azservicebus"
)

// Peek request headers. Like Connection-String, they configure the peek and
// are not message properties.
const (
	headerMaxMessages           = "Max-Messages"
	headerDeadLetter            = "Dead-Letter"
	headerPollInterval          = "Poll-Interval"
	headerBacklog               = "Backlog"
	headerDisplayProperty       = "Display-Property"
	headerTemporarySubscription = "Temporary-Subscription"

	// DefaultMaxMessages is how many messages one peek returns without Max-Messages.
	DefaultMaxMessages = 50
	// DefaultPollInterval is how often a watcher peeks without Poll-Interval.
	DefaultPollInterval = time.Second
	// minPollInterval keeps a watcher from hammering the broker.
	minPollInterval = 100 * time.Millisecond
	// DefaultBacklog is how many of the messages already waiting a watch
	// shows first without Backlog.
	DefaultBacklog = 100

	peekStatusCode = 200
	deadLetterTag  = " (dead-letter)"
	temporaryTag   = " (temporary)"
	// bodyPreviewWidth bounds a message's one-line body preview.
	bodyPreviewWidth = 120
	// emulatorSetting marks an emulator connection string.
	emulatorSetting = "usedevelopmentemulator=true"
)

// peekRequest is a PEEK request with its variables resolved and headers parsed.
type peekRequest struct {
	target           PeekTarget
	connectionString string
	maxMessages      int
	pollInterval     time.Duration
	backlog          int
	displayProperty  string
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
	if t.Temporary {
		name += temporaryTag
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

// SendURL is the URL that sends to the queue or topic a PEEK URL reads, e.g.
// asb://topic for asb://topic/subscription. {{variables}} are kept.
func SendURL(peekURL string) (string, error) {
	target, err := ParsePeekTarget(peekURL)
	if err != nil {
		return "", err
	}
	return urlScheme + target.Entity, nil
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
		backlog:      DefaultBacklog,
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
	if value := request.headers[headerBacklog]; value != "" {
		if request.backlog, err = strconv.Atoi(value); err != nil || request.backlog < 0 {
			return request, fmt.Errorf("%w: %s must be zero or a positive number, got %q", ErrInvalidPeekHeader, headerBacklog, value)
		}
	}
	request.displayProperty = request.headers[headerDisplayProperty]
	if value := request.headers[headerTemporarySubscription]; value != "" {
		if request.target.Temporary, err = strconv.ParseBool(value); err != nil {
			return request, fmt.Errorf("%w: %s must be true or false, got %q", ErrInvalidPeekHeader, headerTemporarySubscription, value)
		}
	}
	return request, validateTemporarySubscription(request)
}

// validateTemporarySubscription checks a temporary subscription is asked for
// on a topic (asb://topic) of a real namespace.
func validateTemporarySubscription(request peekRequest) error {
	if !request.target.Temporary {
		return nil
	}
	if request.target.Subscription != "" || request.target.DeadLetter {
		return fmt.Errorf("%w: %s needs a topic URL such as %s<topic>, without a subscription or %s", ErrInvalidPeekHeader, headerTemporarySubscription, urlScheme, headerDeadLetter)
	}
	if strings.Contains(strings.ToLower(request.connectionString), emulatorSetting) {
		return fmt.Errorf("%w: the emulator only has the subscriptions in its config file; add one there and peek it", ErrTemporarySubscriptionUnsupported)
	}
	return nil
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
//   - Poll-Interval, Backlog, Display-Property, Temporary-Subscription: only
//     used by watches (see Watcher)
//
// The response body is a JSON array of PeekedMessage, oldest first.
func PeekContext(ctx context.Context, req *postman.Request, variables []postman.VariableSource) *http.Response {
	start := time.Now()
	resp := &http.Response{RequestMethod: postman.MethodServiceBusPeek}
	request, err := resolvePeekRequest(req, variables)
	resp.RequestURL = request.url
	resp.RequestHeaders = request.headers
	if err == nil && request.target.Temporary {
		err = fmt.Errorf("%w: a new subscription is empty, so %s only applies to watches", ErrTemporarySubscriptionUnsupported, headerTemporarySubscription)
	}
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
	messages, err := watcher.peekPage(peekCtx, firstSequenceNumber)
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
		DeliveryCount:              message.DeliveryCount,
		MessageID:                  message.MessageID,
		Subject:                    valueOf(message.Subject),
		ContentType:                valueOf(message.ContentType),
		CorrelationID:              valueOf(message.CorrelationID),
		ApplicationProperties:      message.ApplicationProperties,
		DeadLetterReason:           valueOf(message.DeadLetterReason),
		DeadLetterErrorDescription: valueOf(message.DeadLetterErrorDescription),
		DeadLetterSource:           valueOf(message.DeadLetterSource),
		Body:                       jsonBody(message.Body),
	}
	if message.SequenceNumber != nil {
		peeked.SequenceNumber = *message.SequenceNumber
	}
	if message.EnqueuedTime != nil {
		peeked.EnqueuedTime = *message.EnqueuedTime
	}
	// The SDK adds the message's own time-to-live to its enqueued time. A
	// message sent without one carries 0 and expires with the entity's
	// default instead, which a peek doesn't see, so it gets no ExpiresAt.
	if message.ExpiresAt != nil && message.TimeToLive != nil && *message.TimeToLive > 0 {
		peeked.ExpiresAt = *message.ExpiresAt
	}
	return peeked
}

// Label names the message in lists: its displayProperty application
// property, else its subject, else its first application property by key.
func (m PeekedMessage) Label(displayProperty string) string {
	if value, ok := m.ApplicationProperties[displayProperty]; ok && displayProperty != "" {
		return fmt.Sprint(value)
	}
	if m.Subject != "" {
		return m.Subject
	}
	if keys := slices.Sorted(maps.Keys(m.ApplicationProperties)); len(keys) > 0 {
		return fmt.Sprintf("%s=%v", keys[0], m.ApplicationProperties[keys[0]])
	}
	return ""
}

// BodyPreview is the body on one line, compacted when it is JSON and cut
// at bodyPreviewWidth bytes.
func (m PeekedMessage) BodyPreview() string {
	var compact bytes.Buffer
	preview := string(m.Body)
	if json.Compact(&compact, m.Body) == nil {
		preview = compact.String()
	}
	if len(preview) > bodyPreviewWidth {
		preview = preview[:bodyPreviewWidth] + "…"
	}
	return preview
}

// BodyText is the body as sent: JSON as-is, and a text body (peeked as a
// JSON string) unwrapped.
func (m PeekedMessage) BodyText() string {
	var text string
	if json.Unmarshal(m.Body, &text) == nil {
		return text
	}
	return string(m.Body)
}

// SendRequest is a request that sends message again to the queue or topic
// peek reads, connecting with peek's Connection-String as defined, so its
// {{variables}} resolve when it is sent. The message ID is left for the
// broker to assign.
func SendRequest(peek *postman.Request, message PeekedMessage) (*postman.Request, error) {
	rawURL, err := SendURL(peek.URL.Raw)
	if err != nil {
		return nil, err
	}
	request := &postman.Request{
		Method: methodSend,
		URL:    postman.URL{Raw: rawURL},
		Body:   &postman.Body{Mode: postman.BodyModeRaw, Raw: message.BodyText()},
	}
	for _, header := range peek.Header {
		if header.Key == headerConnectionString {
			request.Header = append(request.Header, header)
		}
	}
	if message.Subject != "" {
		request.Header = append(request.Header, postman.Header{Key: headerSubject, Value: message.Subject})
	}
	if message.ContentType != "" {
		request.Header = append(request.Header, postman.Header{Key: headerContentType, Value: message.ContentType})
	}
	for _, key := range slices.Sorted(maps.Keys(message.ApplicationProperties)) {
		request.Header = append(request.Header, postman.Header{Key: key, Value: fmt.Sprint(message.ApplicationProperties[key])})
	}
	return request, nil
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
