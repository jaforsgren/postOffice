package servicebus

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/messaging/azservicebus"
)

var errBrokerDown = errors.New("broker down")

// fakeQueue is a queue or subscription peeked through the peeker interface.
// Its mutex lets a test add messages while a Watch goroutine peeks.
type fakeQueue struct {
	mu       sync.Mutex
	messages []*azservicebus.ReceivedMessage
	// failures is how many of the next peeks fail.
	failures int
	closed   bool
}

func (q *fakeQueue) add(sequence int64, enqueued time.Time) {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.messages = append(q.messages, &azservicebus.ReceivedMessage{SequenceNumber: &sequence, EnqueuedTime: &enqueued, Body: []byte(`{}`)})
}

func (q *fakeQueue) PeekMessages(_ context.Context, maxMessageCount int, options *azservicebus.PeekMessagesOptions) ([]*azservicebus.ReceivedMessage, error) {
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.failures > 0 {
		q.failures--
		return nil, errBrokerDown
	}
	var page []*azservicebus.ReceivedMessage
	for _, message := range q.messages {
		if *message.SequenceNumber >= *options.FromSequenceNumber && len(page) < maxMessageCount {
			page = append(page, message)
		}
	}
	return page, nil
}

func (q *fakeQueue) Close(context.Context) error {
	q.closed = true
	return nil
}

type fakeClient struct{ closed bool }

func (c *fakeClient) Close(context.Context) error {
	c.closed = true
	return nil
}

type fakeSubscriptions struct{ created, deleted []string }

func (s *fakeSubscriptions) createSubscription(_ context.Context, topic, subscription string) error {
	s.created = append(s.created, topic+"/"+subscription)
	return nil
}

func (s *fakeSubscriptions) deleteSubscription(_ context.Context, topic, subscription string) error {
	s.deleted = append(s.deleted, topic+"/"+subscription)
	return nil
}

var watchStart = time.Date(2026, 11, 16, 10, 0, 0, 0, time.UTC)

// fakeWatcher watches queue with a page size of 2, so paging is exercised.
func fakeWatcher(queue *fakeQueue, backlog int) *Watcher {
	return &Watcher{
		request:  peekRequest{target: PeekTarget{Entity: "payments", Subscription: "tap"}, maxMessages: 2, pollInterval: minPollInterval, backlog: backlog},
		client:   &fakeClient{},
		receiver: queue,
	}
}

func sequences(messages []PeekedMessage) []int64 {
	numbers := []int64{}
	for _, message := range messages {
		numbers = append(numbers, message.SequenceNumber)
	}
	return numbers
}

func pollSequences(t *testing.T, watcher *Watcher) []int64 {
	t.Helper()
	messages, err := watcher.Poll(context.Background())
	if err != nil {
		t.Fatalf("Poll() error = %v", err)
	}
	return sequences(messages)
}

func equalSequences(got, want []int64) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range got {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}

func TestWatcher_FirstPollShowsNewestBacklogThenOnlyNew(t *testing.T) {
	queue := &fakeQueue{}
	for sequence := int64(1); sequence <= 5; sequence++ {
		queue.add(sequence, watchStart.Add(time.Duration(sequence)*time.Second))
	}
	watcher := fakeWatcher(queue, 3)

	if got := pollSequences(t, watcher); !equalSequences(got, []int64{3, 4, 5}) {
		t.Errorf("first poll = %v, want the newest 3 of the backlog", got)
	}
	if got := pollSequences(t, watcher); len(got) != 0 {
		t.Errorf("idle poll = %v, want nothing", got)
	}
	queue.add(6, watchStart.Add(time.Minute))
	queue.add(7, watchStart.Add(time.Minute))
	queue.add(8, watchStart.Add(time.Minute))
	if got := pollSequences(t, watcher); !equalSequences(got, []int64{6, 7, 8}) {
		t.Errorf("next poll = %v, want every new message across pages", got)
	}
}

func TestWatcher_ZeroBacklogShowsOnlyNewMessages(t *testing.T) {
	queue := &fakeQueue{}
	queue.add(1, watchStart)
	watcher := fakeWatcher(queue, 0)

	if got := pollSequences(t, watcher); len(got) != 0 {
		t.Errorf("first poll = %v, want the backlog skipped", got)
	}
	queue.add(2, watchStart.Add(time.Second))
	if got := pollSequences(t, watcher); !equalSequences(got, []int64{2}) {
		t.Errorf("next poll = %v, want the new message", got)
	}
}

func TestWatcher_StartsOverWhenSequenceNumbersReset(t *testing.T) {
	queue := &fakeQueue{}
	queue.add(11, watchStart)
	queue.add(12, watchStart.Add(time.Second))
	watcher := fakeWatcher(queue, DefaultBacklog)
	pollSequences(t, watcher)

	// A restarted emulator forgets its messages and numbers new ones from 1.
	queue.messages = nil
	queue.add(1, watchStart.Add(time.Minute))

	if got := pollSequences(t, watcher); !equalSequences(got, []int64{1}) {
		t.Errorf("poll after reset = %v, want the renumbered message", got)
	}
	queue.add(2, watchStart.Add(2*time.Minute))
	if got := pollSequences(t, watcher); !equalSequences(got, []int64{2}) {
		t.Errorf("poll after that = %v, want only the newer message", got)
	}
}

func TestWatcher_OlderMessagesAreNotAReset(t *testing.T) {
	queue := &fakeQueue{}
	queue.add(1, watchStart)
	queue.add(2, watchStart.Add(time.Second))
	watcher := fakeWatcher(queue, DefaultBacklog)
	pollSequences(t, watcher)

	if got := pollSequences(t, watcher); len(got) != 0 {
		t.Errorf("idle poll = %v, want nothing: the oldest message is one already seen", got)
	}
}

func TestWatcher_TemporarySubscriptionLivesAsLongAsTheWatch(t *testing.T) {
	subscriptions := &fakeSubscriptions{}
	watcher := fakeWatcher(&fakeQueue{}, DefaultBacklog)
	watcher.subscriptions = subscriptions

	pollSequences(t, watcher)
	pollSequences(t, watcher)
	watcher.Close()

	if len(subscriptions.created) != 1 || len(subscriptions.deleted) != 1 || subscriptions.created[0] != "payments/tap" {
		t.Errorf("created %v, deleted %v, want payments/tap created once and deleted on close", subscriptions.created, subscriptions.deleted)
	}
}

func TestWatcher_CloseLeavesConfiguredSubscriptions(t *testing.T) {
	queue := &fakeQueue{}
	client := &fakeClient{}
	subscriptions := &fakeSubscriptions{}
	watcher := fakeWatcher(queue, DefaultBacklog)
	watcher.client = client
	watcher.subscriptions = subscriptions

	watcher.Close()

	if !queue.closed || !client.closed || len(subscriptions.deleted) != 0 {
		t.Errorf("receiver closed %v, client closed %v, deleted %v, want both closed and nothing deleted before a poll created it", queue.closed, client.closed, subscriptions.deleted)
	}
}

func TestRetryDelay_DoublesUpToTheCap(t *testing.T) {
	tests := []struct {
		failures int
		want     time.Duration
	}{
		{1, time.Second},
		{2, 2 * time.Second},
		{4, 8 * time.Second},
		{6, maxRetryDelay},
		{1000, maxRetryDelay},
	}
	for _, tt := range tests {
		if got := retryDelay(time.Second, tt.failures); got != tt.want {
			t.Errorf("retryDelay(1s, %d) = %v, want %v", tt.failures, got, tt.want)
		}
	}
}

func TestWatch_ReportsFailuresAndRecoveryButNotIdlePolls(t *testing.T) {
	queue := &fakeQueue{failures: 1}
	watcher := fakeWatcher(queue, DefaultBacklog)
	updates := make(chan WatchUpdate)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go Watch(ctx, watcher, updates)

	failed := <-updates
	if !errors.Is(failed.Err, errBrokerDown) || failed.RetryAt.IsZero() {
		t.Fatalf("first update = %+v, want the failure and when it retries", failed)
	}
	recovered := <-updates
	if recovered.Err != nil || !recovered.RetryAt.IsZero() {
		t.Fatalf("second update = %+v, want the recovery", recovered)
	}

	queue.add(1, watchStart)
	if arrived := <-updates; !equalSequences(sequences(arrived.Messages), []int64{1}) {
		t.Errorf("third update = %+v, want the new message and no idle polls before it", arrived)
	}
	cancel()
	for range updates {
	}
	if !queue.closed {
		t.Error("receiver not closed after the watch stopped")
	}
}
