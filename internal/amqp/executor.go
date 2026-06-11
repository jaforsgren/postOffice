package amqp

import (
	"fmt"
	"postOffice/internal/http"
	"postOffice/internal/postman"
	"strings"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
)

const (
	headerContentType = "Content-Type"
	headerRoutingKey  = "Routing-Key"
	headerExchange    = "Exchange"
	headerMessageID   = "Message-Id"

	defaultExchange    = ""
	defaultContentType = "application/json"
	defaultTimeout     = 30 * time.Second
)

// Publish sends a message to an AMQP broker described by req.
//
// URL format: amqp://host:port/queue or amqps://host:port/queue
// The queue segment of the URL is the default routing key unless
// overridden by a Routing-Key header.
//
// Reserved headers (not sent as AMQP message headers):
//   - Routing-Key: overrides the queue/routing-key from the URL
//   - Exchange: exchange name (defaults to "")
//   - Content-Type: message content type
//   - Message-Id: custom message ID
//
// All other request headers are forwarded as AMQP message headers.
// The request body becomes the message body.
func Publish(req *postman.Request, variables []postman.VariableSource) *http.Response {
	start := time.Now()
	resp := &http.Response{
		RequestMethod: "AMQP",
	}

	rawURL := postman.ResolveVariables(req.URL.Raw, variables)
	resp.RequestURL = rawURL

	resolvedHeaders := make(map[string]string, len(req.Header))
	for _, h := range req.Header {
		resolvedHeaders[h.Key] = postman.ResolveVariables(h.Value, variables)
	}
	resp.RequestHeaders = resolvedHeaders

	brokerURL, routingKey := parseAMQPURL(rawURL)

	if override := resolvedHeaders[headerRoutingKey]; override != "" {
		routingKey = override
	}

	exchange := resolvedHeaders[headerExchange]
	contentType := resolvedHeaders[headerContentType]
	if contentType == "" {
		contentType = defaultContentType
	}

	var body string
	if req.Body != nil {
		body = postman.ResolveVariables(req.Body.Raw, variables)
	}
	resp.RequestBody = body

	conn, err := amqp.DialConfig(brokerURL, amqp.Config{
		Dial: amqp.DefaultDial(defaultTimeout),
	})
	if err != nil {
		resp.Error = fmt.Errorf("failed to connect to AMQP broker: %w", err)
		resp.Duration = time.Since(start)
		return resp
	}
	defer conn.Close()

	ch, err := conn.Channel()
	if err != nil {
		resp.Error = fmt.Errorf("failed to open AMQP channel: %w", err)
		resp.Duration = time.Since(start)
		return resp
	}
	defer ch.Close()

	msgHeaders := buildAMQPHeaders(resolvedHeaders)

	publishing := amqp.Publishing{
		ContentType:  contentType,
		Body:         []byte(body),
		Headers:      msgHeaders,
		DeliveryMode: amqp.Persistent,
	}

	if msgID := resolvedHeaders[headerMessageID]; msgID != "" {
		publishing.MessageId = msgID
	}

	if err := ch.Publish(exchange, routingKey, false, false, publishing); err != nil {
		resp.Error = fmt.Errorf("failed to publish message to %q: %w", routingKey, err)
		resp.Duration = time.Since(start)
		return resp
	}

	resp.Duration = time.Since(start)
	resp.StatusCode = 200
	resp.Status = "Message Published"
	resp.Body = fmt.Sprintf("Message published to exchange=%q routing-key=%q in %v",
		exchange, routingKey, resp.Duration.Round(time.Millisecond))
	return resp
}

// parseAMQPURL splits an amqp(s)://host:port/queue URL into a dial URL and routing key.
// The returned dial URL uses the amqp(s) scheme without the path segment.
func parseAMQPURL(rawURL string) (brokerURL, routingKey string) {
	// Find scheme
	schemeEnd := strings.Index(rawURL, "://")
	if schemeEnd < 0 {
		return rawURL, ""
	}

	afterScheme := rawURL[schemeEnd+3:]
	slashIdx := strings.Index(afterScheme, "/")
	if slashIdx < 0 {
		return rawURL, ""
	}

	scheme := rawURL[:schemeEnd]
	host := afterScheme[:slashIdx]
	routingKey = strings.TrimPrefix(afterScheme[slashIdx:], "/")

	// Rebuild broker URL without routing key path for dialing
	brokerURL = scheme + "://" + host
	return brokerURL, routingKey
}

// buildAMQPHeaders converts request headers to amqp.Table,
// excluding reserved headers that map to first-class message fields.
func buildAMQPHeaders(headers map[string]string) amqp.Table {
	reserved := map[string]bool{
		headerContentType: true,
		headerRoutingKey:  true,
		headerExchange:    true,
		headerMessageID:   true,
	}

	table := amqp.Table{}
	for k, v := range headers {
		if !reserved[k] {
			table[k] = v
		}
	}
	return table
}
