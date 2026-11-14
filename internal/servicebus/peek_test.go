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
	if request.maxMessages != DefaultMaxMessages || request.pollInterval != DefaultPollInterval {
		t.Errorf("max = %d, interval = %v, want the defaults", request.maxMessages, request.pollInterval)
	}
}

func TestResolvePeekRequest_ParsesOptionalHeaders(t *testing.T) {
	request, err := resolvePeekRequest(peekRequestTo("asb://orders",
		postman.Header{Key: headerMaxMessages, Value: "5"},
		postman.Header{Key: headerDeadLetter, Value: "true"},
		postman.Header{Key: headerPollInterval, Value: "250ms"},
	), testVariables)
	if err != nil {
		t.Fatalf("resolvePeekRequest() error = %v", err)
	}
	if request.maxMessages != 5 || !request.target.DeadLetter || request.pollInterval != 250*time.Millisecond {
		t.Errorf("request = %+v, want the header values", request)
	}
}

func TestResolvePeekRequest_RejectsBadHeaders(t *testing.T) {
	for _, header := range []postman.Header{
		{Key: headerMaxMessages, Value: "0"},
		{Key: headerMaxMessages, Value: "many"},
		{Key: headerDeadLetter, Value: "maybe"},
		{Key: headerPollInterval, Value: "1ms"},
		{Key: headerPollInterval, Value: "soon"},
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
	subject := "LF-26-NWGXJJ"
	message := &azservicebus.ReceivedMessage{
		MessageID:             "m1",
		SequenceNumber:        &sequence,
		EnqueuedTime:          &enqueued,
		Subject:               &subject,
		ApplicationProperties: map[string]any{"message-name": "claim.created.v1"},
		Body:                  []byte(`{"id":"LF-26-NWGXJJ"}`),
	}

	peeked := newPeekedMessage(message)

	if peeked.SequenceNumber != sequence || !peeked.EnqueuedTime.Equal(enqueued) || peeked.Subject != subject || peeked.MessageID != "m1" {
		t.Errorf("peeked = %+v", peeked)
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
