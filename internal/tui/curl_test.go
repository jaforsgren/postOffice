package tui

import (
	"postOffice/internal/postman"
	"strings"
	"testing"
)

func TestParseCurlToItem_SimpleGet(t *testing.T) {
	item, err := parseCurlToItem(`curl https://api.example.com/users`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if item.Request.Method != "GET" {
		t.Errorf("expected method GET, got %s", item.Request.Method)
	}
	if item.Request.URL.Raw != "https://api.example.com/users" {
		t.Errorf("unexpected URL: %s", item.Request.URL.Raw)
	}
	if item.Name != "users" {
		t.Errorf("expected name 'users', got %s", item.Name)
	}
}

func TestParseCurlToItem_MethodHeadersAndBody(t *testing.T) {
	cmd := `curl -X POST 'https://api.example.com/users' -H 'Content-Type: application/json' -H 'Authorization: Bearer abc123' -d '{"name":"Alice"}'`

	item, err := parseCurlToItem(cmd)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if item.Request.Method != "POST" {
		t.Errorf("expected method POST, got %s", item.Request.Method)
	}
	if len(item.Request.Header) != 2 {
		t.Fatalf("expected 2 headers, got %d", len(item.Request.Header))
	}
	if item.Request.Header[0].Key != "Content-Type" || item.Request.Header[0].Value != "application/json" {
		t.Errorf("unexpected header[0]: %+v", item.Request.Header[0])
	}
	if item.Request.Body == nil || item.Request.Body.Raw != `{"name":"Alice"}` {
		t.Errorf("unexpected body: %+v", item.Request.Body)
	}
}

func TestParseCurlToItem_DataImpliesPost(t *testing.T) {
	item, err := parseCurlToItem(`curl https://api.example.com/users -d 'name=Alice'`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if item.Request.Method != "POST" {
		t.Errorf("expected -d without -X to imply POST, got %s", item.Request.Method)
	}
}

func TestParseCurlToItem_ExplicitMethodNotOverriddenByData(t *testing.T) {
	item, err := parseCurlToItem(`curl -X PUT https://api.example.com/users/1 -d '{"name":"Bob"}'`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if item.Request.Method != "PUT" {
		t.Errorf("expected method to remain PUT, got %s", item.Request.Method)
	}
}

func TestParseCurlToItem_IgnoresKnownFlagsAndAuth(t *testing.T) {
	cmd := `curl -s -L -k -u user:pass https://api.example.com/users`
	item, err := parseCurlToItem(cmd)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if item.Request.URL.Raw != "https://api.example.com/users" {
		t.Errorf("unexpected URL: %s", item.Request.URL.Raw)
	}
}

func TestParseCurlToItem_NotACurlCommand(t *testing.T) {
	if _, err := parseCurlToItem("wget https://example.com"); err == nil {
		t.Fatal("expected error for non-curl command")
	}
}

func TestParseCurlToItem_MissingURL(t *testing.T) {
	if _, err := parseCurlToItem("curl -H 'X-Test: 1'"); err == nil {
		t.Fatal("expected error when no URL is present")
	}
}

func TestParseGRPCCurlToItem_Basic(t *testing.T) {
	cmd := `grpcurl -plaintext -d '{"name":"world"}' localhost:50051 helloworld.Greeter/SayHello`

	item, err := parseGRPCCurlToItem(cmd)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if item.Request.Method != "GRPC" {
		t.Errorf("expected method GRPC, got %s", item.Request.Method)
	}
	if item.Request.URL.Raw != "grpc://localhost:50051/helloworld.Greeter/SayHello" {
		t.Errorf("unexpected URL: %s", item.Request.URL.Raw)
	}
	if item.Name != "SayHello" {
		t.Errorf("expected name SayHello, got %s", item.Name)
	}
	if item.Request.Body == nil || item.Request.Body.Raw != `{"name":"world"}` {
		t.Errorf("unexpected body: %+v", item.Request.Body)
	}
}

func TestParseGRPCCurlToItem_TLSByDefault(t *testing.T) {
	item, err := parseGRPCCurlToItem(`grpcurl localhost:50051 helloworld.Greeter/SayHello`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.HasPrefix(item.Request.URL.Raw, "grpcs://") {
		t.Errorf("expected grpcs:// scheme by default, got %s", item.Request.URL.Raw)
	}
}

func TestParseGRPCCurlToItem_MissingMethod(t *testing.T) {
	if _, err := parseGRPCCurlToItem("grpcurl -plaintext localhost:50051"); err == nil {
		t.Fatal("expected error when host/method is incomplete")
	}
}

func TestParseGRPCCurlToItem_NotAGRPCCurlCommand(t *testing.T) {
	if _, err := parseGRPCCurlToItem("curl https://example.com"); err == nil {
		t.Fatal("expected error for non-grpcurl command")
	}
}

func TestDetectCurlType(t *testing.T) {
	tests := []struct {
		name string
		text string
		want string
	}{
		{"curl command", "curl https://example.com", "curl"},
		{"curl with leading whitespace", "  curl https://example.com", "curl"},
		{"grpcurl command", "grpcurl -plaintext localhost:50051 Foo/Bar", "grpcurl"},
		{"unrelated text", "just some notes", ""},
		{"empty string", "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := detectCurlType(tt.text); got != tt.want {
				t.Errorf("expected %q, got %q", tt.want, got)
			}
		})
	}
}

func TestTokenizeShell_QuotesAndEscapes(t *testing.T) {
	tokens := tokenizeShell(`curl -H 'X-Test: a b' -d "{\"a\":1}" 'https://example.com/x y'`)
	want := []string{"curl", "-H", "X-Test: a b", "-d", `{"a":1}`, "https://example.com/x y"}

	if len(tokens) != len(want) {
		t.Fatalf("expected %d tokens, got %d: %v", len(want), len(tokens), tokens)
	}
	for i := range want {
		if tokens[i] != want[i] {
			t.Errorf("token %d: expected %q, got %q", i, want[i], tokens[i])
		}
	}
}

func TestTokenizeShell_LineContinuation(t *testing.T) {
	tokens := tokenizeShell("curl \\\n  -X POST \\\n  https://example.com")
	want := []string{"curl", "-X", "POST", "https://example.com"}

	if len(tokens) != len(want) {
		t.Fatalf("expected %d tokens, got %d: %v", len(want), len(tokens), tokens)
	}
}

func TestExtractNameFromURL(t *testing.T) {
	tests := []struct {
		url  string
		want string
	}{
		{"https://api.example.com/users", "users"},
		{"https://api.example.com/users/", "users"},
		{"https://api.example.com/users?active=true", "users"},
		{"https://api.example.com/", "api.example.com"},
	}
	for _, tt := range tests {
		if got := extractNameFromURL(tt.url); got != tt.want {
			t.Errorf("extractNameFromURL(%q) = %q, want %q", tt.url, got, tt.want)
		}
	}
}

func TestHandlePasteCurlKey_NoCollectionLoaded(t *testing.T) {
	parser := postman.NewParser()
	m := NewModel(parser)
	m.mode = ModeRequests

	m, _ = handlePasteCurlKey(m)

	if m.statusMessage != "Load a collection first" {
		t.Errorf("unexpected status message: %q", m.statusMessage)
	}
}

func TestInsertItemAtCursor_ParsedCurlItem(t *testing.T) {
	path := writeTempCollectionFile(t, "Curl Test Collection")
	parser := postman.NewParser()
	collection, err := parser.LoadCollection(path)
	if err != nil {
		t.Fatalf("failed to load collection: %v", err)
	}

	m := NewModel(parser)
	m.collection = collection
	m.mode = ModeRequests
	m = m.loadRequestsList()

	item, err := parseCurlToItem(`curl -X POST 'https://api.example.com/users' -d '{"name":"Alice"}'`)
	if err != nil {
		t.Fatalf("unexpected parse error: %v", err)
	}

	m = m.insertItemAtCursor(*item)

	if len(m.collection.Items) != 1 {
		t.Fatalf("expected 1 item in collection, got %d", len(m.collection.Items))
	}
	inserted := m.collection.Items[0]
	if inserted.Name != "users" || inserted.Request.Method != "POST" {
		t.Errorf("unexpected inserted item: %+v", inserted)
	}
}

func TestBuildCurlCommand_FormDataBecomesFormArguments(t *testing.T) {
	req := &postman.Request{
		Method: "POST",
		URL:    postman.URL{Raw: "{{base}}/attachments"},
		Body: &postman.Body{Mode: postman.BodyModeFormData, FormData: []postman.FormParam{
			{Key: "scope", Value: "{{scope}}", Type: "text"},
			{Key: "hidden", Value: "x", Type: "text", Disabled: true},
			{Key: "file", Type: postman.FormParamTypeFile, Src: []byte(`["/tmp/it's.png","/tmp/b.pdf"]`), ContentType: "image/png"},
		}},
	}
	vars := []postman.VariableSource{{Key: "base", Value: "https://api.example.com"}, {Key: "scope", Value: "Customer"}}

	cmd := buildCurlCommand(req, vars)

	for _, want := range []string{
		"-F 'scope=Customer'",
		`-F 'file=@/tmp/it'\''s.png;type=image/png'`,
		"-F 'file=@/tmp/b.pdf;type=image/png'",
	} {
		if !strings.Contains(cmd, want) {
			t.Errorf("expected %q in:\n%s", want, cmd)
		}
	}
	if strings.Contains(cmd, "hidden") || strings.Contains(cmd, "-d ") {
		t.Errorf("expected no disabled field and no -d body:\n%s", cmd)
	}
}
