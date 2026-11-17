package servicebus

import (
	"context"
	"fmt"
	"postOffice/internal/postman"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/messaging/azservicebus"
)

const (
	// firstSequenceNumber peeks from the oldest message.
	firstSequenceNumber int64 = 0
	// maxRetryDelay caps how long a failing watch waits between polls.
	maxRetryDelay = 30 * time.Second
	// closeTimeout bounds deleting a temporary subscription and closing the
	// connection when a watch stops.
	closeTimeout = 10 * time.Second
)

// peeker is the part of a receiver a watcher uses; *azservicebus.Receiver
// implements it.
type peeker interface {
	PeekMessages(ctx context.Context, maxMessageCount int, options *azservicebus.PeekMessagesOptions) ([]*azservicebus.ReceivedMessage, error)
	Close(ctx context.Context) error
}

type closer interface {
	Close(ctx context.Context) error
}

// Watcher peeks a queue or subscription repeatedly, each time returning only
// the messages that arrived since the previous peek. A Watcher is not safe
// for concurrent use.
type Watcher struct {
	request  peekRequest
	client   closer
	receiver peeker
	// subscriptions is set for a temporary subscription, which the first
	// Poll creates and Close deletes.
	subscriptions       subscriptionManager
	subscriptionCreated bool
	position            peekPosition
}

// peekPosition is where a watcher's next poll starts. The watcher tracks it
// itself, rather than leaving it to the receiver, so it can start over when
// the sequence numbers do.
type peekPosition struct {
	started      bool
	next         int64
	lastEnqueued time.Time
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
	var subscriptions subscriptionManager
	if request.target.Temporary {
		manager, err := newAdminSubscriptions(request.connectionString)
		if err != nil {
			return nil, err
		}
		subscriptions = manager
		request.target.Subscription = newTemporarySubscriptionName()
	}

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
	return &Watcher{request: request, client: client, receiver: receiver, subscriptions: subscriptions}, nil
}

// Target is the queue or subscription the watcher peeks.
func (w *Watcher) Target() PeekTarget {
	return w.request.target
}

// PollInterval is how long to wait between polls.
func (w *Watcher) PollInterval() time.Duration {
	return w.request.pollInterval
}

// DisplayProperty is the application property that labels a message in
// lists (the Display-Property header), empty when not set.
func (w *Watcher) DisplayProperty() string {
	return w.request.displayProperty
}

// Poll returns every message that arrived since the previous poll, oldest
// first. The first poll returns the newest Backlog messages already waiting.
func (w *Watcher) Poll(ctx context.Context) ([]PeekedMessage, error) {
	if err := w.ensureSubscription(ctx); err != nil {
		return nil, err
	}
	if !w.position.started {
		messages, err := w.peekFrom(ctx, firstSequenceNumber)
		if err != nil {
			return nil, err
		}
		w.position.started = true
		w.advance(messages)
		return newest(messages, w.request.backlog), nil
	}

	messages, err := w.peekFrom(ctx, w.position.next)
	if err != nil || len(messages) > 0 {
		w.advance(messages)
		return messages, err
	}
	reset, err := w.sequenceReset(ctx)
	if err != nil || !reset {
		return nil, err
	}
	messages, err = w.peekFrom(ctx, firstSequenceNumber)
	w.advance(messages)
	return messages, err
}

// peekFrom peeks every message from sequence number from on, a page of
// Max-Messages at a time. On an error it returns the pages read so far.
func (w *Watcher) peekFrom(ctx context.Context, from int64) ([]PeekedMessage, error) {
	var messages []PeekedMessage
	for {
		page, err := w.peekPage(ctx, from)
		messages = append(messages, page...)
		if err != nil || len(page) < w.request.maxMessages {
			return messages, err
		}
		from = page[len(page)-1].SequenceNumber + 1
	}
}

// peekPage peeks up to Max-Messages messages from sequence number from on.
func (w *Watcher) peekPage(ctx context.Context, from int64) ([]PeekedMessage, error) {
	received, err := w.receiver.PeekMessages(ctx, w.request.maxMessages, &azservicebus.PeekMessagesOptions{FromSequenceNumber: &from})
	if err != nil {
		return nil, fmt.Errorf("failed to peek %q: %w", w.request.target, err)
	}
	messages := make([]PeekedMessage, len(received))
	for i, message := range received {
		messages[i] = newPeekedMessage(message)
	}
	return messages, nil
}

func (w *Watcher) advance(messages []PeekedMessage) {
	if len(messages) == 0 {
		return
	}
	last := messages[len(messages)-1]
	w.position.next = last.SequenceNumber + 1
	w.position.lastEnqueued = last.EnqueuedTime
}

// sequenceReset reports whether the entity's sequence numbers started over,
// as the emulator's do when it restarts: its oldest message then has a
// sequence number the watcher already passed, but was enqueued after the
// newest message the watcher saw. It costs one extra peek per idle poll.
func (w *Watcher) sequenceReset(ctx context.Context) (bool, error) {
	if w.position.lastEnqueued.IsZero() {
		return false, nil
	}
	from := firstSequenceNumber
	oldest, err := w.receiver.PeekMessages(ctx, 1, &azservicebus.PeekMessagesOptions{FromSequenceNumber: &from})
	if err != nil {
		return false, fmt.Errorf("failed to peek %q: %w", w.request.target, err)
	}
	if len(oldest) == 0 {
		return false, nil
	}
	message := newPeekedMessage(oldest[0])
	return message.SequenceNumber < w.position.next && message.EnqueuedTime.After(w.position.lastEnqueued), nil
}

func (w *Watcher) ensureSubscription(ctx context.Context) error {
	if w.subscriptions == nil || w.subscriptionCreated {
		return nil
	}
	if err := w.subscriptions.createSubscription(ctx, w.request.target.Entity, w.request.target.Subscription); err != nil {
		return fmt.Errorf("failed to create temporary subscription %q: %w", w.request.target, err)
	}
	w.subscriptionCreated = true
	return nil
}

// Close deletes the watcher's temporary subscription, if it created one, and
// releases its connection.
func (w *Watcher) Close() {
	ctx, cancel := context.WithTimeout(context.Background(), closeTimeout)
	defer cancel()
	_ = w.receiver.Close(ctx)
	if w.subscriptionCreated {
		_ = w.subscriptions.deleteSubscription(ctx, w.request.target.Entity, w.request.target.Subscription)
	}
	_ = w.client.Close(ctx)
}

// Watch polls watcher until ctx is cancelled and sends what it finds on
// updates: polls that found messages or failed, and the first good poll
// after a failure, so the error clears. Idle polls send nothing. After each
// failure in a row the watch waits twice as long, up to maxRetryDelay. Watch
// closes watcher and updates when it returns.
func Watch(ctx context.Context, watcher *Watcher, updates chan<- WatchUpdate) {
	defer close(updates)
	defer watcher.Close()
	failures := 0
	for {
		messages, err := watcher.Poll(ctx)
		if ctx.Err() != nil {
			return
		}
		recovered := err == nil && failures > 0
		update := WatchUpdate{Messages: messages, Err: err}
		delay := watcher.PollInterval()
		if err != nil {
			failures++
			delay = retryDelay(delay, failures)
			update.RetryAt = time.Now().Add(delay)
		} else {
			failures = 0
		}
		if len(messages) > 0 || err != nil || recovered {
			select {
			case updates <- update:
			case <-ctx.Done():
				return
			}
		}
		select {
		case <-time.After(delay):
		case <-ctx.Done():
			return
		}
	}
}

// retryDelay is how long to wait after failures polls in a row failed:
// interval doubled for each failure after the first, up to maxRetryDelay.
func retryDelay(interval time.Duration, failures int) time.Duration {
	delay := interval
	for range failures - 1 {
		if delay >= maxRetryDelay {
			break
		}
		delay *= 2
	}
	return min(delay, maxRetryDelay)
}

// newest returns the last count messages.
func newest(messages []PeekedMessage, count int) []PeekedMessage {
	return messages[max(len(messages)-count, 0):]
}
