package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"postOffice/internal/postman"
	"postOffice/internal/servicebus"
)

const (
	watchConnection = "{{connection}}"
	// emulatorConnectionVariable opts in to running against the Service Bus
	// emulator, as in the servicebus package's tests.
	emulatorConnectionVariable = "POSTOFFICE_SERVICEBUS_EMULATOR"
	emulatorDeliveryTimeout    = 10 * time.Second
)

func peekItem(name, url string) postman.Item {
	return postman.Item{Name: name, Request: &postman.Request{
		Method: postman.MethodServiceBusPeek,
		URL:    postman.URL{Raw: url},
		Header: []postman.Header{{Key: "Connection-String", Value: watchConnection}},
	}}
}

// eventsCollection has PEEK requests at the top and in a Topics folder, and
// an ASB send that watch ignores.
func eventsCollection() *postman.Collection {
	return &postman.Collection{
		Info: postman.Info{Name: "Events"},
		Items: []postman.Item{
			peekItem("orders", "asb://orders"),
			{Name: "Topics", Items: []postman.Item{
				peekItem("payments", "asb://payments/postoffice-tap"),
				{Name: "send payment", Request: &postman.Request{Method: "ASB", URL: postman.URL{Raw: "asb://payments"}}},
			}},
		},
	}
}

func refPaths(t *testing.T, collection *postman.Collection, requestPath string, folderPath []string) []string {
	t.Helper()
	refs, err := peekRequestsToWatch(collection, requestPath, folderPath)
	if err != nil {
		t.Fatalf("peekRequestsToWatch() error = %v", err)
	}
	paths := make([]string, len(refs))
	for i, ref := range refs {
		paths[i] = ref.Path
	}
	return paths
}

func TestParseWatchArgs(t *testing.T) {
	opts, err := parseWatchArgs([]string{"events.json", "--folder", "Topics", "-e", "local", "--output", "json"})
	if err != nil {
		t.Fatal(err)
	}
	if opts.collectionPath != "events.json" || fmt.Sprint(opts.folderPath) != "[Topics]" || opts.execution.environmentPath != "local" || opts.execution.output != outputJSON {
		t.Errorf("opts = %+v, want the collection, folder, environment and output", opts)
	}
}

func TestParseWatchArgs_Rejects(t *testing.T) {
	for _, args := range [][]string{
		{"events.json", "Topics/payments", "--folder", "Topics"},
		{"events.json", "--output", "junit"},
		{"events.json", "a", "b"},
		{"events.json", "--persist"},
	} {
		if _, err := parseWatchArgs(args); !errors.Is(err, errUsage) {
			t.Errorf("parseWatchArgs(%q) error = %v, want a usage error", args, err)
		}
	}
}

func TestPeekRequestsToWatch(t *testing.T) {
	collection := eventsCollection()
	tests := []struct {
		name        string
		requestPath string
		folderPath  []string
		want        string
	}{
		{"collection", "", nil, "[orders Topics/payments]"},
		{"folder", "", []string{"Topics"}, "[Topics/payments]"},
		{"request", "orders", nil, "[orders]"},
	}
	for _, tt := range tests {
		if got := fmt.Sprint(refPaths(t, collection, tt.requestPath, tt.folderPath)); got != tt.want {
			t.Errorf("%s: watched %s, want %s", tt.name, got, tt.want)
		}
	}
}

func TestPeekRequestsToWatch_NoneFound(t *testing.T) {
	for _, requestPath := range []string{"Topics/send payment", "missing"} {
		if _, err := peekRequestsToWatch(eventsCollection(), requestPath, nil); !errors.Is(err, errNoPeekRequests) {
			t.Errorf("%s: error = %v, want errNoPeekRequests", requestPath, err)
		}
	}
}

func TestOpenWatchers_ReportsUnresolvedVariables(t *testing.T) {
	collection := eventsCollection()
	refs, _ := peekRequestsToWatch(collection, "orders", nil)

	_, _, err := openWatchers(collection, nil, refs)

	if err == nil || !strings.Contains(err.Error(), watchConnection) {
		t.Errorf("error = %v, want the unresolved connection variable named", err)
	}
}

func TestWriteWatchUpdate(t *testing.T) {
	request := watchedRequest{name: "Topics/payments", target: "payments/postoffice-tap", displayProperty: "message-name"}
	message := servicebus.PeekedMessage{SequenceNumber: 7, ApplicationProperties: map[string]any{"message-name": "payment.completed.v1"}, Body: json.RawMessage(`{"id": "LF-1"}`)}
	failure := servicebus.WatchUpdate{Err: errors.New("broker down"), RetryAt: time.Date(2026, 11, 16, 10, 4, 31, 0, time.Local)}

	tests := []struct {
		name   string
		output outputFormat
		update servicebus.WatchUpdate
		want   []string
	}{
		{"text message", outputText, servicebus.WatchUpdate{Messages: []servicebus.PeekedMessage{message}}, []string{"payments/postoffice-tap  #7  payment.completed.v1  {\"id\":\"LF-1\"}"}},
		{"text failure", outputText, failure, []string{"error: broker down (retrying at 10:04:31)"}},
		{"text recovery", outputText, servicebus.WatchUpdate{}, []string{"polling again"}},
		{"json message", outputJSON, servicebus.WatchUpdate{Messages: []servicebus.PeekedMessage{message}}, []string{`"request":"Topics/payments"`, `"sequenceNumber":7`, `"body":{"id":"LF-1"}`}},
		{"json failure", outputJSON, failure, []string{`"error":"broker down"`, `"retryAt":`}},
	}
	for _, tt := range tests {
		var out bytes.Buffer
		if err := writeWatchUpdate(&out, tt.output, watchedUpdate{request: request, update: tt.update}); err != nil {
			t.Fatal(err)
		}
		for _, want := range tt.want {
			if !strings.Contains(out.String(), want) {
				t.Errorf("%s: wrote %q, want it to contain %q", tt.name, out.String(), want)
			}
		}
	}
}

func TestWriteWatchUpdate_JSONRecoveryWritesNothing(t *testing.T) {
	var out bytes.Buffer
	if err := writeWatchUpdate(&out, outputJSON, watchedUpdate{update: servicebus.WatchUpdate{}}); err != nil || out.Len() != 0 {
		t.Errorf("wrote %q, %v, want nothing: json lines are messages and failures", out.String(), err)
	}
}

func TestEmulator_WatchStreamsSentMessage(t *testing.T) {
	connection := os.Getenv(emulatorConnectionVariable)
	if connection == "" {
		t.Skipf("set %s to run against the Service Bus emulator", emulatorConnectionVariable)
	}
	dir := t.TempDir()
	collectionPath := writeJSONFile(t, dir, "events.postman_collection.json", &postman.Collection{
		Info:      postman.Info{Name: "Events"},
		Items:     []postman.Item{peekItem("payments", "asb://payments/postoffice-tap")},
		Variables: []postman.Variable{{Key: "connection", Value: connection}},
	})
	ctx, cancel := context.WithTimeout(context.Background(), emulatorDeliveryTimeout)
	defer cancel()
	messageID := fmt.Sprintf("postoffice-cli-test-%d", time.Now().UnixNano())
	out := &messageWaiter{messageID: messageID, found: cancel}
	opts := runOptions{collectionPath: collectionPath, requestPath: "payments", execution: executionOptions{output: outputJSON}}

	go func() {
		time.Sleep(time.Second)
		send := &postman.Request{Method: "ASB", URL: postman.URL{Raw: "asb://payments"}, Header: []postman.Header{{Key: "Connection-String", Value: connection}, {Key: "Message-Id", Value: messageID}}, Body: &postman.Body{Mode: postman.BodyModeRaw, Raw: `{}`}}
		servicebus.SendContext(context.Background(), send, nil)
	}()
	if err := executeWatch(ctx, out, opts); err != nil {
		t.Fatal(err)
	}
	if !out.seen {
		t.Errorf("the sent message never streamed; wrote:\n%s", out.String())
	}
}

// messageWaiter collects watch output and calls found once messageID shows up.
type messageWaiter struct {
	bytes.Buffer
	messageID string
	found     context.CancelFunc
	seen      bool
}

func (w *messageWaiter) Write(p []byte) (int, error) {
	n, err := w.Buffer.Write(p)
	if strings.Contains(w.String(), w.messageID) {
		w.seen = true
		w.found()
	}
	return n, err
}
