package servicebus

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"postOffice/internal/postman"
	"strings"
	"testing"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/messaging/azservicebus"
)

const (
	// emulatorConnectionVariable opts in to the tests that need a running
	// Service Bus emulator (see develophub-docker), e.g.
	// Endpoint=sb://localhost;SharedAccessKeyName=RootManageSharedAccessKey;SharedAccessKey=SAS_KEY_VALUE;UseDevelopmentEmulator=true;
	emulatorConnectionVariable = "POSTOFFICE_SERVICEBUS_EMULATOR"
	emulatorTopic              = "payments"
	emulatorTapSubscription    = "postoffice-tap"
	// emulatorDeliveryTimeout bounds the emulator's lag in delivering a sent
	// message to its topic's subscriptions.
	emulatorDeliveryTimeout = 10 * time.Second
	emulatorRetryInterval   = 200 * time.Millisecond
	testConnectionString    = "Endpoint=sb://localhost;SharedAccessKeyName=key;SharedAccessKey=secret;UseDevelopmentEmulator=true;"
)

func peekRequestTo(rawURL string, headers ...postman.Header) *postman.Request {
	return &postman.Request{
		Method: postman.MethodServiceBusPeek,
		URL:    postman.URL{Raw: rawURL},
		Header: append([]postman.Header{{Key: headerConnectionString, Value: "{{connection}}"}}, headers...),
	}
}

var testVariables = []postman.VariableSource{{Key: "connection", Value: testConnectionString}, {Key: "topic", Value: "payments"}}

// azureVariables connect to a namespace rather than the emulator. Nothing in
// the unit tests dials it.
var azureVariables = []postman.VariableSource{{Key: "connection", Value: "Endpoint=sb://example.servicebus.windows.net/;SharedAccessKeyName=key;SharedAccessKey=secret"}}

func TestParsePeekTarget(t *testing.T) {
	tests := []struct {
		url  string
		want PeekTarget
	}{
		{"asb://orders", PeekTarget{Entity: "orders"}},
		{"ASB://Payments/PostOffice-Tap", PeekTarget{Entity: "Payments", Subscription: "PostOffice-Tap"}},
	}
	for _, tt := range tests {
		got, err := ParsePeekTarget(tt.url)
		if err != nil || got != tt.want {
			t.Errorf("ParsePeekTarget(%q) = %+v, %v, want %+v", tt.url, got, err, tt.want)
		}
	}
}

func TestParsePeekTarget_Rejects(t *testing.T) {
	for _, url := range []string{"https://payments", "asb://", "asb://payments/", "asb:///tap", "asb://payments/subscriptions/tap"} {
		if _, err := ParsePeekTarget(url); !errors.Is(err, ErrInvalidPeekTarget) {
			t.Errorf("ParsePeekTarget(%q) error = %v, want ErrInvalidPeekTarget", url, err)
		}
	}
}

func TestPeekTarget_String(t *testing.T) {
	target := PeekTarget{Entity: "payments", Subscription: "tap", DeadLetter: true}
	if got := target.String(); got != "payments/tap"+deadLetterTag {
		t.Errorf("String() = %q", got)
	}
}

func TestResolvePeekRequest_ResolvesVariablesAndDefaults(t *testing.T) {
	request, err := resolvePeekRequest(peekRequestTo("asb://{{topic}}/tap"), testVariables)
	if err != nil {
		t.Fatalf("resolvePeekRequest() error = %v", err)
	}
	if request.target != (PeekTarget{Entity: "payments", Subscription: "tap"}) || request.connectionString != testConnectionString {
		t.Errorf("request = %+v, want the resolved target and connection string", request)
	}
	if request.maxMessages != DefaultMaxMessages || request.pollInterval != DefaultPollInterval || request.backlog != DefaultBacklog || request.displayProperty != "" {
		t.Errorf("request = %+v, want the defaults", request)
	}
}

func TestResolvePeekRequest_ParsesOptionalHeaders(t *testing.T) {
	request, err := resolvePeekRequest(peekRequestTo("asb://orders",
		postman.Header{Key: headerMaxMessages, Value: "5"},
		postman.Header{Key: headerDeadLetter, Value: "true"},
		postman.Header{Key: headerPollInterval, Value: "250ms"},
		postman.Header{Key: headerBacklog, Value: "0"},
		postman.Header{Key: headerDisplayProperty, Value: "message-name"},
	), testVariables)
	if err != nil {
		t.Fatalf("resolvePeekRequest() error = %v", err)
	}
	if request.maxMessages != 5 || !request.target.DeadLetter || request.pollInterval != 250*time.Millisecond || request.backlog != 0 || request.displayProperty != "message-name" {
		t.Errorf("request = %+v, want the header values", request)
	}
}

func TestResolvePeekRequest_TemporarySubscriptionOnATopic(t *testing.T) {
	request, err := resolvePeekRequest(peekRequestTo("asb://payments", postman.Header{Key: headerTemporarySubscription, Value: "true"}), azureVariables)
	if err != nil {
		t.Fatalf("resolvePeekRequest() error = %v", err)
	}
	if request.target != (PeekTarget{Entity: "payments", Temporary: true}) {
		t.Errorf("target = %+v, want a temporary subscription on payments", request.target)
	}
}

func TestResolvePeekRequest_RejectsTemporarySubscriptionMisuse(t *testing.T) {
	temporary := postman.Header{Key: headerTemporarySubscription, Value: "true"}
	tests := []struct {
		name      string
		req       *postman.Request
		variables []postman.VariableSource
		want      error
	}{
		{"named subscription", peekRequestTo("asb://payments/tap", temporary), azureVariables, ErrInvalidPeekHeader},
		{"dead-letter", peekRequestTo("asb://payments", temporary, postman.Header{Key: headerDeadLetter, Value: "true"}), azureVariables, ErrInvalidPeekHeader},
		{"emulator", peekRequestTo("asb://payments", temporary), testVariables, ErrTemporarySubscriptionUnsupported},
	}
	for _, tt := range tests {
		if _, err := resolvePeekRequest(tt.req, tt.variables); !errors.Is(err, tt.want) {
			t.Errorf("%s: error = %v, want %v", tt.name, err, tt.want)
		}
	}
}

func TestPeekContext_RejectsTemporarySubscription(t *testing.T) {
	resp := PeekContext(context.Background(), peekRequestTo("asb://payments", postman.Header{Key: headerTemporarySubscription, Value: "true"}), azureVariables)

	if !errors.Is(resp.Error, ErrTemporarySubscriptionUnsupported) {
		t.Errorf("error = %v, want ErrTemporarySubscriptionUnsupported", resp.Error)
	}
}

func TestPeekTarget_StringTagsTemporary(t *testing.T) {
	target := PeekTarget{Entity: "payments", Subscription: "postoffice-1a2b3c4d", Temporary: true}
	if got := target.String(); got != "payments/postoffice-1a2b3c4d"+temporaryTag {
		t.Errorf("String() = %q", got)
	}
}

func TestNewTemporarySubscriptionName_IsUnique(t *testing.T) {
	first, second := newTemporarySubscriptionName(), newTemporarySubscriptionName()
	if first == second || !strings.HasPrefix(first, temporarySubscriptionPrefix) {
		t.Errorf("names %q and %q, want two distinct %s names", first, second, temporarySubscriptionPrefix)
	}
}

func TestResolvePeekRequest_RejectsBadHeaders(t *testing.T) {
	for _, header := range []postman.Header{
		{Key: headerMaxMessages, Value: "0"},
		{Key: headerMaxMessages, Value: "many"},
		{Key: headerDeadLetter, Value: "maybe"},
		{Key: headerPollInterval, Value: "1ms"},
		{Key: headerPollInterval, Value: "soon"},
		{Key: headerBacklog, Value: "-1"},
		{Key: headerBacklog, Value: "all"},
		{Key: headerTemporarySubscription, Value: "maybe"},
	} {
		if _, err := resolvePeekRequest(peekRequestTo("asb://orders", header), testVariables); !errors.Is(err, ErrInvalidPeekHeader) {
			t.Errorf("%s: %q error = %v, want ErrInvalidPeekHeader", header.Key, header.Value, err)
		}
	}
}

func TestPeekContext_MissingConnectionStringFails(t *testing.T) {
	req := &postman.Request{Method: postman.MethodServiceBusPeek, URL: postman.URL{Raw: "asb://payments/tap"}}

	resp := PeekContext(context.Background(), req, nil)

	if resp.Error == nil || !strings.Contains(resp.Error.Error(), headerConnectionString) {
		t.Errorf("error = %v, want a missing %s error", resp.Error, headerConnectionString)
	}
	if resp.RequestMethod != postman.MethodServiceBusPeek || resp.RequestURL != "asb://payments/tap" {
		t.Errorf("recorded %s %s, want the peek request", resp.RequestMethod, resp.RequestURL)
	}
}

func TestNewPeekedMessage(t *testing.T) {
	sequence := int64(7)
	enqueued := time.Date(2026, 11, 14, 10, 0, 0, 0, time.UTC)
	timeToLive := time.Hour
	expires := enqueued.Add(timeToLive)
	subject := "LF-26-NWGXJJ"
	reason, description, source := "MaxDeliveryCountExceeded", "Message could not be consumed after 10 delivery attempts.", "payments/sejfa-internal"
	message := &azservicebus.ReceivedMessage{
		MessageID:                  "m1",
		SequenceNumber:             &sequence,
		EnqueuedTime:               &enqueued,
		ExpiresAt:                  &expires,
		TimeToLive:                 &timeToLive,
		DeliveryCount:              10,
		Subject:                    &subject,
		ApplicationProperties:      map[string]any{"message-name": "claim.created.v1"},
		DeadLetterReason:           &reason,
		DeadLetterErrorDescription: &description,
		DeadLetterSource:           &source,
		Body:                       []byte(`{"id":"LF-26-NWGXJJ"}`),
	}

	peeked := newPeekedMessage(message)

	if peeked.SequenceNumber != sequence || !peeked.EnqueuedTime.Equal(enqueued) || peeked.Subject != subject || peeked.MessageID != "m1" {
		t.Errorf("peeked = %+v", peeked)
	}
	if !peeked.ExpiresAt.Equal(expires) || peeked.DeliveryCount != 10 || peeked.DeadLetterReason != reason || peeked.DeadLetterErrorDescription != description || peeked.DeadLetterSource != source {
		t.Errorf("peeked = %+v, want the delivery and dead-letter fields", peeked)
	}
	if string(peeked.Body) != `{"id":"LF-26-NWGXJJ"}` {
		t.Errorf("body = %s, want the JSON body as-is", peeked.Body)
	}
}

func TestJSONBody_EncodesNonJSONAsString(t *testing.T) {
	for body, want := range map[string]string{"plain text": `"plain text"`, "": `""`} {
		if got := string(jsonBody([]byte(body))); got != want {
			t.Errorf("jsonBody(%q) = %s, want %s", body, got, want)
		}
	}
}

func emulatorConnection(t *testing.T) string {
	t.Helper()
	connection := os.Getenv(emulatorConnectionVariable)
	if connection == "" {
		t.Skipf("set %s to run against the Service Bus emulator", emulatorConnectionVariable)
	}
	return connection
}

func sendTestMessage(t *testing.T, connection, messageID string) {
	t.Helper()
	req := &postman.Request{
		Method: "ASB",
		URL:    postman.URL{Raw: urlScheme + emulatorTopic},
		Header: []postman.Header{{Key: headerConnectionString, Value: connection}, {Key: headerMessageID, Value: messageID}},
		Body:   &postman.Body{Mode: postman.BodyModeRaw, Raw: fmt.Sprintf(`{"id":%q}`, messageID)},
	}
	if resp := SendContext(context.Background(), req, nil); resp.Error != nil {
		t.Fatalf("send: %v", resp.Error)
	}
}

func TestEmulator_WatcherSeesOnlyNewMessages(t *testing.T) {
	connection := emulatorConnection(t)
	watcher, err := OpenWatcher(peekRequestTo(urlScheme+emulatorTopic+"/"+emulatorTapSubscription), []postman.VariableSource{{Key: "connection", Value: connection}})
	if err != nil {
		t.Fatalf("OpenWatcher() error = %v", err)
	}
	defer watcher.Close()
	if _, err := watcher.Poll(context.Background()); err != nil {
		t.Fatalf("draining poll: %v", err)
	}

	messageID := fmt.Sprintf("postoffice-test-%d", time.Now().UnixNano())
	sendTestMessage(t, connection, messageID)
	var messages []PeekedMessage
	eventually(t, func() bool {
		polled, err := watcher.Poll(context.Background())
		if err != nil {
			t.Fatalf("Poll() error = %v", err)
		}
		messages = append(messages, polled...)
		return len(messages) > 0
	})

	if len(messages) != 1 || messages[0].MessageID != messageID {
		t.Fatalf("polled %+v, want just the message sent", messages)
	}
	var body struct{ ID string }
	if err := json.Unmarshal(messages[0].Body, &body); err != nil || body.ID != messageID {
		t.Errorf("body = %s, want the sent JSON", messages[0].Body)
	}
}

func TestEmulator_PeekContextReturnsWaitingMessages(t *testing.T) {
	connection := emulatorConnection(t)
	messageID := fmt.Sprintf("postoffice-test-%d", time.Now().UnixNano())
	sendTestMessage(t, connection, messageID)
	req := peekRequestTo(urlScheme+emulatorTopic+"/"+emulatorTapSubscription, postman.Header{Key: headerMaxMessages, Value: "250"})

	eventually(t, func() bool {
		resp := PeekContext(context.Background(), req, []postman.VariableSource{{Key: "connection", Value: connection}})
		if resp.Error != nil || resp.StatusCode != peekStatusCode {
			t.Fatalf("PeekContext() = %d %v", resp.StatusCode, resp.Error)
		}
		return strings.Contains(resp.Body, messageID)
	})
}

// eventually retries delivered until it reports true, failing the test after
// emulatorDeliveryTimeout.
func eventually(t *testing.T, delivered func() bool) {
	t.Helper()
	deadline := time.Now().Add(emulatorDeliveryTimeout)
	for !delivered() {
		if time.Now().After(deadline) {
			t.Fatalf("message not delivered within %v", emulatorDeliveryTimeout)
		}
		time.Sleep(emulatorRetryInterval)
	}
}

func TestSendURL_DropsTheSubscription(t *testing.T) {
	tests := map[string]string{
		"asb://{{sb_topic_payments}}/postoffice-tap": "asb://{{sb_topic_payments}}",
		"asb://orders": "asb://orders",
	}
	for peekURL, want := range tests {
		if got, err := SendURL(peekURL); err != nil || got != want {
			t.Errorf("SendURL(%q) = %q, %v, want %q", peekURL, got, err, want)
		}
	}
	if _, err := SendURL("https://payments"); !errors.Is(err, ErrInvalidPeekTarget) {
		t.Errorf("SendURL(https) error = %v, want ErrInvalidPeekTarget", err)
	}
}

func TestSendRequest_SendsTheMessageAgain(t *testing.T) {
	peek := peekRequestTo("asb://{{topic}}/tap", postman.Header{Key: headerMaxMessages, Value: "5"})
	message := PeekedMessage{
		MessageID:             "original-id",
		Subject:               "LF-1",
		ContentType:           "application/json",
		ApplicationProperties: map[string]any{"z-last": 1, "message-name": "claim.created.v1"},
		Body:                  json.RawMessage(`{"id":"LF-1"}`),
	}

	request, err := SendRequest(peek, message)
	if err != nil {
		t.Fatal(err)
	}

	want := []postman.Header{
		{Key: headerConnectionString, Value: "{{connection}}"},
		{Key: headerSubject, Value: "LF-1"},
		{Key: headerContentType, Value: "application/json"},
		{Key: "message-name", Value: "claim.created.v1"},
		{Key: "z-last", Value: "1"},
	}
	if request.Method != methodSend || request.URL.Raw != "asb://{{topic}}" || request.Body.Raw != `{"id":"LF-1"}` {
		t.Errorf("request = %s %s %q, want an ASB send of the body to the topic", request.Method, request.URL.Raw, request.Body.Raw)
	}
	if fmt.Sprint(request.Header) != fmt.Sprint(want) {
		t.Errorf("headers = %v, want %v (no peek headers, no message ID)", request.Header, want)
	}
}

func TestBodyText_UnwrapsTextBodies(t *testing.T) {
	for body, want := range map[string]string{`"plain text"`: "plain text", `{"a":1}`: `{"a":1}`} {
		if got := (PeekedMessage{Body: json.RawMessage(body)}).BodyText(); got != want {
			t.Errorf("BodyText(%s) = %q, want %q", body, got, want)
		}
	}
}

func TestBodyPreview(t *testing.T) {
	if got := (PeekedMessage{Body: json.RawMessage("{\n  \"a\": 1\n}")}).BodyPreview(); got != `{"a":1}` {
		t.Errorf("BodyPreview(JSON) = %q, want it compacted", got)
	}
	long := PeekedMessage{Body: json.RawMessage(`"` + strings.Repeat("x", bodyPreviewWidth*2) + `"`)}
	if got := long.BodyPreview(); len(got) > bodyPreviewWidth+len("…") {
		t.Errorf("BodyPreview(long) is %d bytes, want it truncated", len(got))
	}
}

func TestNewPeekedMessage_NoExpiryWithoutItsOwnTimeToLive(t *testing.T) {
	enqueued := time.Date(2026, 11, 14, 10, 0, 0, 0, time.UTC)
	noTimeToLive := time.Duration(0)
	message := &azservicebus.ReceivedMessage{EnqueuedTime: &enqueued, ExpiresAt: &enqueued, TimeToLive: &noTimeToLive}

	if peeked := newPeekedMessage(message); !peeked.ExpiresAt.IsZero() {
		t.Errorf("ExpiresAt = %v, want none: the entity's default time-to-live applies", peeked.ExpiresAt)
	}
}
