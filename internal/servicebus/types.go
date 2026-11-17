package servicebus

import (
	"context"
	"encoding/json"
	"errors"
	"time"
)

var (
	// ErrInvalidPeekTarget is returned for a PEEK URL that names neither a
	// queue (asb://queue) nor a topic subscription (asb://topic/subscription).
	ErrInvalidPeekTarget = errors.New("invalid peek target")
	// ErrInvalidPeekHeader is returned for a peek header whose value can't be parsed.
	ErrInvalidPeekHeader = errors.New("invalid peek header")
	// ErrTemporarySubscriptionUnsupported is returned when a temporary
	// subscription can't be used: on the emulator, which only has the
	// subscriptions in its config file, or for a one-shot peek, which would
	// always find the new subscription empty.
	ErrTemporarySubscriptionUnsupported = errors.New("temporary subscription unsupported")
)

// PeekTarget is the queue or topic subscription a peek reads.
type PeekTarget struct {
	// Entity is the queue, or the topic when Subscription is set.
	Entity       string
	Subscription string
	// DeadLetter peeks the entity's dead-letter queue instead.
	DeadLetter bool
	// Temporary marks a subscription the watcher creates on the topic and
	// deletes when it closes.
	Temporary bool
}

// PeekedMessage is a message read by a peek. Peeking neither locks nor
// removes it, so the subscription's consumers still receive it.
type PeekedMessage struct {
	SequenceNumber        int64          `json:"sequenceNumber"`
	EnqueuedTime          time.Time      `json:"enqueuedTime"`
	ExpiresAt             time.Time      `json:"expiresAt,omitzero"`
	DeliveryCount         uint32         `json:"deliveryCount"`
	MessageID             string         `json:"messageId,omitempty"`
	Subject               string         `json:"subject,omitempty"`
	ContentType           string         `json:"contentType,omitempty"`
	CorrelationID         string         `json:"correlationId,omitempty"`
	ApplicationProperties map[string]any `json:"applicationProperties,omitempty"`
	// The dead-letter fields are set on messages peeked from a dead-letter queue.
	DeadLetterReason           string `json:"deadLetterReason,omitempty"`
	DeadLetterErrorDescription string `json:"deadLetterErrorDescription,omitempty"`
	DeadLetterSource           string `json:"deadLetterSource,omitempty"`
	// Body is a JSON body as-is, any other body as a JSON string, so scripts
	// can read pm.response.json()[0].body.someField.
	Body json.RawMessage `json:"body"`
}

// SourcedMessage is a peeked message with the PEEK request that saw it and
// the queue or subscription it was peeked from, as exported or streamed.
type SourcedMessage struct {
	Request string `json:"request"`
	Target  string `json:"target"`
	PeekedMessage
}

// WatchUpdate is what one poll of a watch found: new messages, or the error
// it hit and when the watch tries again.
type WatchUpdate struct {
	Messages []PeekedMessage
	Err      error
	// RetryAt is when the next poll runs after a failed one.
	RetryAt time.Time
}

// subscriptionManager creates and deletes the temporary subscriptions of
// watches. The admin client implements it; tests fake it.
type subscriptionManager interface {
	createSubscription(ctx context.Context, topic, subscription string) error
	deleteSubscription(ctx context.Context, topic, subscription string) error
}
