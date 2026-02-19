package tui

import "testing"

func TestParseGRPCURL(t *testing.T) {
	tests := []struct {
		name            string
		rawURL          string
		expectedHost    string
		expectedService string
		expectedMethod  string
	}{
		{
			name:            "Full URL with package, service, and method",
			rawURL:          "grpc://localhost:50051/helloworld.Greeter/SayHello",
			expectedHost:    "localhost:50051",
			expectedService: "helloworld.Greeter",
			expectedMethod:  "SayHello",
		},
		{
			name:            "URL with service and method, no package",
			rawURL:          "grpc://api.example.com/UserService/GetUser",
			expectedHost:    "api.example.com",
			expectedService: "UserService",
			expectedMethod:  "GetUser",
		},
		{
			name:            "Host only, no path",
			rawURL:          "grpc://localhost:50051",
			expectedHost:    "localhost:50051",
			expectedService: "",
			expectedMethod:  "",
		},
		{
			name:            "Host with single path segment",
			rawURL:          "grpc://localhost:50051/ServiceOnly",
			expectedHost:    "localhost:50051",
			expectedService: "ServiceOnly",
			expectedMethod:  "",
		},
		{
			name:            "Variable in URL",
			rawURL:          "grpc://{{grpcHost}}/pkg.Service/Method",
			expectedHost:    "{{grpcHost}}",
			expectedService: "pkg.Service",
			expectedMethod:  "Method",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			host, service, method := parseGRPCURL(tt.rawURL)
			if host != tt.expectedHost {
				t.Errorf("host: expected %q, got %q", tt.expectedHost, host)
			}
			if service != tt.expectedService {
				t.Errorf("service: expected %q, got %q", tt.expectedService, service)
			}
			if method != tt.expectedMethod {
				t.Errorf("method: expected %q, got %q", tt.expectedMethod, method)
			}
		})
	}
}
