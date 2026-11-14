package servicebus

import (
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
)

// PeekTarget is the queue or topic subscription a peek reads.
type PeekTarget struct {
	// Entity is the queue, or the topic when Subscription is set.
	Entity       string
	Subscription string
	// DeadLetter peeks the entity's dead-letter queue instead.
	DeadLetter bool
}

// PeekedMessage is a message read by a peek. Peeking neither locks nor
// removes it, so the subscription's consumers still receive it.
type PeekedMessage struct {
	SequenceNumber        int64          `json:"sequenceNumber"`
	EnqueuedTime          time.Time      `json:"enqueuedTime"`
	MessageID             string         `json:"messageId,omitempty"`
	Subject               string         `json:"subject,omitempty"`
	ContentType           string         `json:"contentType,omitempty"`
	CorrelationID         string         `json:"correlationId,omitempty"`
	ApplicationProperties map[string]any `json:"applicationProperties,omitempty"`
	// Body is a JSON body as-is, any other body as a JSON string, so scripts
	// can read pm.response.json()[0].body.someField.
	Body json.RawMessage `json:"body"`
}
