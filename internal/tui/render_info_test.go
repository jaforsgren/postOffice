package tui

import "testing"

func TestParseGRPCURL(t *testing.T) {
	tests := []struct {
		name            string
		rawURL          string
		expectedHost    string
		expectedService string
		expectedMethod  string
		expectedTLS     bool
	}{
		{
			name:            "insecure full URL",
			rawURL:          "grpc://localhost:50051/helloworld.Greeter/SayHello",
			expectedHost:    "localhost:50051",
			expectedService: "helloworld.Greeter",
			expectedMethod:  "SayHello",
			expectedTLS:     false,
		},
		{
			name:            "TLS full URL",
			rawURL:          "grpcs://api.example.com/helloworld.Greeter/SayHello",
			expectedHost:    "api.example.com",
			expectedService: "helloworld.Greeter",
			expectedMethod:  "SayHello",
			expectedTLS:     true,
		},
		{
			name:            "URL with service and method, no package",
			rawURL:          "grpc://api.example.com/UserService/GetUser",
			expectedHost:    "api.example.com",
			expectedService: "UserService",
			expectedMethod:  "GetUser",
			expectedTLS:     false,
		},
		{
			name:            "host only, no path",
			rawURL:          "grpc://localhost:50051",
			expectedHost:    "localhost:50051",
			expectedService: "",
			expectedMethod:  "",
			expectedTLS:     false,
		},
		{
			name:            "TLS host only",
			rawURL:          "grpcs://localhost:50051",
			expectedHost:    "localhost:50051",
			expectedService: "",
			expectedMethod:  "",
			expectedTLS:     true,
		},
		{
			name:            "host with single path segment",
			rawURL:          "grpc://localhost:50051/ServiceOnly",
			expectedHost:    "localhost:50051",
			expectedService: "ServiceOnly",
			expectedMethod:  "",
			expectedTLS:     false,
		},
		{
			name:            "variable in URL",
			rawURL:          "grpc://{{grpcHost}}/pkg.Service/Method",
			expectedHost:    "{{grpcHost}}",
			expectedService: "pkg.Service",
			expectedMethod:  "Method",
			expectedTLS:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			host, service, method, tls := parseGRPCURL(tt.rawURL)
			if host != tt.expectedHost {
				t.Errorf("host: expected %q, got %q", tt.expectedHost, host)
			}
			if service != tt.expectedService {
				t.Errorf("service: expected %q, got %q", tt.expectedService, service)
			}
			if method != tt.expectedMethod {
				t.Errorf("method: expected %q, got %q", tt.expectedMethod, method)
			}
			if tls != tt.expectedTLS {
				t.Errorf("tls: expected %v, got %v", tt.expectedTLS, tls)
			}
		})
	}
}
