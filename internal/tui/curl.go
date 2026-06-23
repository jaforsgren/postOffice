package tui

import (
	"fmt"
	"postOffice/internal/postman"
	"strings"
)

// buildCurlCommand generates a curl command with variables resolved.
func buildCurlCommand(req *postman.Request, vars []postman.VariableSource) string {
	url := postman.ResolveVariables(req.URL.Raw, vars)

	var parts []string
	parts = append(parts, "curl")

	if req.Method != "" && req.Method != "GET" {
		parts = append(parts, "-X "+req.Method)
	}

	for _, h := range req.Header {
		val := postman.ResolveVariables(h.Value, vars)
		parts = append(parts, fmt.Sprintf("-H '%s: %s'", h.Key, val))
	}

	if req.Body != nil && req.Body.Raw != "" {
		body := postman.ResolveVariables(req.Body.Raw, vars)
		body = strings.ReplaceAll(body, "'", `'\''`)
		parts = append(parts, fmt.Sprintf("-d '%s'", body))
	}

	parts = append(parts, fmt.Sprintf("'%s'", url))

	return strings.Join(parts, " \\\n  ")
}

// buildGRPCCurlCommand generates a grpcurl command with variables resolved.
func buildGRPCCurlCommand(item postman.Item, vars []postman.VariableSource) string {
	req := item.Request
	rawURL := postman.ResolveVariables(req.URL.Raw, vars)

	endpoint, service, method, tls := parseGRPCURL(rawURL)
	fullMethod := service
	if method != "" {
		fullMethod = service + "/" + method
	}

	var parts []string
	parts = append(parts, "grpcurl")

	if !tls {
		parts = append(parts, "-plaintext")
	}

	for _, h := range req.Header {
		val := postman.ResolveVariables(h.Value, vars)
		parts = append(parts, fmt.Sprintf("-H '%s: %s'", h.Key, val))
	}

	if req.Body != nil && req.Body.Raw != "" {
		body := postman.ResolveVariables(req.Body.Raw, vars)
		body = strings.ReplaceAll(body, "'", `'\''`)
		parts = append(parts, fmt.Sprintf("-d '%s'", body))
	}

	parts = append(parts, endpoint)
	if fullMethod != "" {
		parts = append(parts, fullMethod)
	}

	return strings.Join(parts, " \\\n  ")
}

// parseCurlToItem parses a curl command into a postman.Item.
func parseCurlToItem(text string) (*postman.Item, error) {
	tokens := tokenizeShell(strings.TrimSpace(text))
	if len(tokens) == 0 || tokens[0] != "curl" {
		return nil, fmt.Errorf("not a curl command")
	}

	req := &postman.Request{
		Method: "GET",
		Header: []postman.Header{},
	}
	var rawURL string

	for i := 1; i < len(tokens); i++ {
		tok := tokens[i]
		switch {
		case tok == "-X" || tok == "--request":
			if i+1 < len(tokens) {
				i++
				req.Method = strings.ToUpper(tokens[i])
			}
		case strings.HasPrefix(tok, "-X"):
			req.Method = strings.ToUpper(tok[2:])
		case tok == "-H" || tok == "--header":
			if i+1 < len(tokens) {
				i++
				parts := strings.SplitN(tokens[i], ":", 2)
				if len(parts) == 2 {
					req.Header = append(req.Header, postman.Header{
						Key:   strings.TrimSpace(parts[0]),
						Value: strings.TrimSpace(parts[1]),
					})
				}
			}
		case tok == "-d" || tok == "--data" || tok == "--data-raw" || tok == "--data-binary":
			if i+1 < len(tokens) {
				i++
				req.Body = &postman.Body{Mode: "raw", Raw: tokens[i]}
				if req.Method == "GET" {
					req.Method = "POST"
				}
			}
		case tok == "-u" || tok == "--user":
			if i+1 < len(tokens) {
				i++ // skip value
			}
		case isKnownCurlFlag(tok):
			// ignore known single-token flags
		case !strings.HasPrefix(tok, "-"):
			rawURL = tok
		}
	}

	if rawURL == "" {
		return nil, fmt.Errorf("no URL found in curl command")
	}

	req.URL.Raw = rawURL

	return &postman.Item{
		Name:    extractNameFromURL(rawURL),
		Request: req,
	}, nil
}

// parseGRPCCurlToItem parses a grpcurl command into a postman.Item.
func parseGRPCCurlToItem(text string) (*postman.Item, error) {
	tokens := tokenizeShell(strings.TrimSpace(text))
	if len(tokens) == 0 || tokens[0] != "grpcurl" {
		return nil, fmt.Errorf("not a grpcurl command")
	}

	req := &postman.Request{
		Method: "GRPC",
		Header: []postman.Header{},
	}

	tls := true
	var positional []string

	for i := 1; i < len(tokens); i++ {
		tok := tokens[i]
		switch {
		case tok == "-plaintext" || tok == "--plaintext":
			tls = false
		case tok == "-insecure" || tok == "--insecure":
			tls = false
		case tok == "-H" || tok == "--header" || tok == "-rpc-header":
			if i+1 < len(tokens) {
				i++
				parts := strings.SplitN(tokens[i], ":", 2)
				if len(parts) == 2 {
					req.Header = append(req.Header, postman.Header{
						Key:   strings.TrimSpace(parts[0]),
						Value: strings.TrimSpace(parts[1]),
					})
				}
			}
		case tok == "-d" || tok == "--data":
			if i+1 < len(tokens) {
				i++
				req.Body = &postman.Body{Mode: "raw", Raw: tokens[i]}
			}
		case tok == "-import-path" || tok == "-proto" || tok == "-authority" || tok == "-connect-timeout" || tok == "-max-time":
			i++ // skip value
		case !strings.HasPrefix(tok, "-"):
			positional = append(positional, tok)
		}
	}

	if len(positional) < 2 {
		return nil, fmt.Errorf("grpcurl command missing host and method")
	}

	endpoint := positional[len(positional)-2]
	fullMethod := positional[len(positional)-1]

	scheme := "grpcs"
	if !tls {
		scheme = "grpc"
	}
	req.URL.Raw = scheme + "://" + endpoint + "/" + fullMethod

	parts := strings.Split(fullMethod, "/")
	name := parts[len(parts)-1]
	if name == "" {
		name = fullMethod
	}

	return &postman.Item{
		Name:    name,
		Request: req,
	}, nil
}

// detectCurlType returns "grpcurl", "curl", or "" based on the clipboard text.
func detectCurlType(text string) string {
	trimmed := strings.TrimSpace(text)
	if strings.HasPrefix(trimmed, "grpcurl") {
		return "grpcurl"
	}
	if strings.HasPrefix(trimmed, "curl") {
		return "curl"
	}
	return ""
}

// tokenizeShell splits a shell command into tokens respecting single/double quotes and backslash continuation.
func tokenizeShell(s string) []string {
	s = strings.ReplaceAll(s, "\\\r\n", " ")
	s = strings.ReplaceAll(s, "\\\n", " ")

	var tokens []string
	var current strings.Builder
	inSingle := false
	inDouble := false

	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case inSingle:
			if c == '\'' {
				inSingle = false
			} else {
				current.WriteByte(c)
			}
		case inDouble:
			if c == '"' {
				inDouble = false
			} else if c == '\\' && i+1 < len(s) {
				i++
				next := s[i]
				switch next {
				case '"', '\\', '$', '`', '\n':
					current.WriteByte(next)
				default:
					current.WriteByte('\\')
					current.WriteByte(next)
				}
			} else {
				current.WriteByte(c)
			}
		case c == '\'':
			inSingle = true
		case c == '"':
			inDouble = true
		case c == '\\' && i+1 < len(s):
			i++
			current.WriteByte(s[i])
		case c == ' ' || c == '\t' || c == '\n' || c == '\r':
			if current.Len() > 0 {
				tokens = append(tokens, current.String())
				current.Reset()
			}
		default:
			current.WriteByte(c)
		}
	}

	if current.Len() > 0 {
		tokens = append(tokens, current.String())
	}

	return tokens
}

func extractNameFromURL(rawURL string) string {
	u := rawURL
	if idx := strings.Index(u, "://"); idx != -1 {
		u = u[idx+3:]
	}
	if idx := strings.Index(u, "?"); idx != -1 {
		u = u[:idx]
	}
	parts := strings.Split(strings.TrimRight(u, "/"), "/")
	for i := len(parts) - 1; i >= 0; i-- {
		if parts[i] != "" {
			return parts[i]
		}
	}
	return "New Request"
}

func isKnownCurlFlag(tok string) bool {
	knownFlags := []string{
		"-L", "--location", "-s", "--silent", "-v", "--verbose",
		"-k", "--insecure", "-i", "--include", "-I", "--head",
		"-f", "--fail", "-S", "--show-error", "-g", "--globoff",
		"--compressed", "--no-buffer", "-0", "--http1.0",
		"-1", "--tlsv1", "-2", "--sslv2", "-3", "--sslv3",
	}
	for _, f := range knownFlags {
		if tok == f {
			return true
		}
	}
	return false
}
