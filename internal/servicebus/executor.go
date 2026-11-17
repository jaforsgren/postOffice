package servicebus

import (
	"context"
	"fmt"
	"postOffice/internal/http"
	"postOffice/internal/postman"
	"strings"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/messaging/azservicebus"
)

const (
	headerConnectionString = "Connection-String"
	headerSubject          = "Subject"
	headerContentType      = "Content-Type"
	headerMessageID        = "Message-Id"

	defaultTimeout = 30 * time.Second

	methodSend = "ASB"

	urlScheme = "asb://"
)

// SendContext delivers a message to an Azure Service Bus topic or queue described by req.
//
// URL format: asb://topic-or-queue
// Reserved headers (not sent as application properties):
//   - Connection-String: Azure Service Bus connection string (required)
//   - Subject: message subject/label
//   - Content-Type: message content type
//   - Message-Id: custom message ID
//
// All other request headers are sent as application properties.
// The request body becomes the message body. The send is cancelled when ctx is.
func SendContext(ctx context.Context, req *postman.Request, variables []postman.VariableSource) *http.Response {
	start := time.Now()
	resp := &http.Response{
		RequestMethod: methodSend,
	}

	rawURL := postman.ResolveVariables(req.URL.Raw, variables)
	topicOrQueue, _ := entityPath(rawURL)
	resp.RequestURL = rawURL

	resolvedHeaders := resolveHeaders(req, variables)
	resp.RequestHeaders = resolvedHeaders

	connectionString := resolvedHeaders[headerConnectionString]
	if connectionString == "" {
		resp.Error = fmt.Errorf("missing required header: %s", headerConnectionString)
		resp.Duration = time.Since(start)
		return resp
	}

	var body string
	if req.Body != nil {
		body = postman.ResolveVariables(req.Body.Raw, variables)
	}
	resp.RequestBody = body

	appProperties := buildAppProperties(resolvedHeaders)

	client, err := azservicebus.NewClientFromConnectionString(connectionString, nil)
	if err != nil {
		resp.Error = fmt.Errorf("failed to create Service Bus client: %w", err)
		resp.Duration = time.Since(start)
		return resp
	}
	defer client.Close(context.Background())

	sender, err := client.NewSender(topicOrQueue, nil)
	if err != nil {
		resp.Error = fmt.Errorf("failed to create sender for %q: %w", topicOrQueue, err)
		resp.Duration = time.Since(start)
		return resp
	}
	defer sender.Close(context.Background())

	msg := &azservicebus.Message{
		Body:                  []byte(body),
		ApplicationProperties: appProperties,
	}

	if subject := resolvedHeaders[headerSubject]; subject != "" {
		msg.Subject = &subject
	}
	if ct := resolvedHeaders[headerContentType]; ct != "" {
		msg.ContentType = &ct
	}
	if msgID := resolvedHeaders[headerMessageID]; msgID != "" {
		msg.MessageID = &msgID
	}

	sendCtx, cancel := context.WithTimeout(ctx, defaultTimeout)
	defer cancel()

	if err := sender.SendMessage(sendCtx, msg, nil); err != nil {
		resp.Error = fmt.Errorf("failed to send message to %q: %w", topicOrQueue, err)
		resp.Duration = time.Since(start)
		return resp
	}

	resp.Duration = time.Since(start)
	resp.StatusCode = 200
	resp.Status = "Message Sent"
	resp.Body = fmt.Sprintf("Message delivered to %q in %v", topicOrQueue, resp.Duration.Round(time.Millisecond))
	return resp
}

// entityPath is the resolved URL without its case-insensitive asb:// scheme,
// keeping the entity's own casing. ok is false without the scheme.
func entityPath(rawURL string) (path string, ok bool) {
	if !strings.HasPrefix(strings.ToLower(rawURL), urlScheme) {
		return "", false
	}
	return rawURL[len(urlScheme):], true
}

func resolveHeaders(req *postman.Request, variables []postman.VariableSource) map[string]string {
	resolved := make(map[string]string, len(req.Header))
	for _, header := range req.Header {
		resolved[header.Key] = postman.ResolveVariables(header.Value, variables)
	}
	return resolved
}

// buildAppProperties converts request headers to Service Bus application properties,
// excluding reserved headers that map to first-class message fields.
func buildAppProperties(headers map[string]string) map[string]any {
	reserved := map[string]bool{
		headerConnectionString: true,
		headerSubject:          true,
		headerContentType:      true,
		headerMessageID:        true,
	}

	props := make(map[string]any)
	for k, v := range headers {
		if !reserved[k] {
			props[k] = v
		}
	}
	return props
}
