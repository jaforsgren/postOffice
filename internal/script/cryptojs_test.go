package script

import (
	"postOffice/internal/postman"
	"testing"
)

func TestDecodeBase64(t *testing.T) {
	cases := []struct {
		name    string
		encoded string
		want    string
	}{
		{name: "padded", encoded: "YWI=", want: "ab"},
		{name: "unpadded", encoded: "YWI", want: "ab"},
		{name: "no padding needed", encoded: "YWJj", want: "abc"},
		{name: "whitespace", encoded: " YW\nI= ", want: "ab"},
		{name: "empty", encoded: "", want: ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := decodeBase64(tc.encoded)
			if err != nil {
				t.Fatalf("decodeBase64(%q) error: %v", tc.encoded, err)
			}
			if string(got) != tc.want {
				t.Errorf("decodeBase64(%q) = %q, want %q", tc.encoded, got, tc.want)
			}
		})
	}
}

func TestDecodeBase64_RejectsInvalidInput(t *testing.T) {
	for _, encoded := range []string{"Y", "YW!=", "YW-_"} {
		if _, err := decodeBase64(encoded); err == nil {
			t.Errorf("decodeBase64(%q) succeeded, want an error", encoded)
		}
	}
}

// The JWT payload {"persona_id":"admin"} base64url-encodes to 30 characters, so it carries no
// padding - the shape that broke JWT decoding in collection scripts.
const unpaddedJWT = "eyJhbGciOiJIUzI1NiJ9.eyJwZXJzb25hX2lkIjoiYWRtaW4ifQ.c2lnbmF0dXJl"

func TestBase64DecodesUnpaddedJWTPayload(t *testing.T) {
	cases := []struct {
		name   string
		decode string
	}{
		{name: "CryptoJS.enc.Base64.parse", decode: "CryptoJS.enc.Base64.parse(segment).toString(CryptoJS.enc.Utf8)"},
		{name: "atob", decode: "atob(segment)"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			script := postman.Script{
				Type: "text/javascript",
				Exec: []string{
					"const segment = '" + unpaddedJWT + "'.split('.')[1].replace(/-/g, '+').replace(/_/g, '/');",
					"pm.test('decodes payload', () => {",
					"    pm.expect(JSON.parse(" + tc.decode + ").persona_id).to.eql('admin');",
					"});",
				},
			}

			result := NewRuntime().ExecuteTestScript(script, &ExecutionContext{Response: &ResponseData{StatusCode: 200}})

			if len(result.Tests) != 1 || !result.Tests[0].Passed {
				t.Fatalf("expected the decode test to pass, got %+v", result.Tests)
			}
		})
	}
}

func TestWordArrayToString(t *testing.T) {
	cases := []struct {
		name       string
		expression string
		want       string
	}{
		{name: "defaults to hex", expression: "CryptoJS.enc.Utf8.parse('ab').toString()", want: "6162"},
		{name: "utf8 encoder", expression: "CryptoJS.enc.Base64.parse('YWI').toString(CryptoJS.enc.Utf8)", want: "ab"},
		{name: "base64 encoder", expression: "CryptoJS.enc.Utf8.parse('ab').toString(CryptoJS.enc.Base64)", want: "YWI="},
		{name: "hex round trip", expression: "CryptoJS.enc.Hex.parse('6162').toString(CryptoJS.enc.Utf8)", want: "ab"},
		{name: "hex stringify", expression: "CryptoJS.enc.Hex.stringify(CryptoJS.enc.Utf8.parse('ab'))", want: "6162"},
		{name: "utf8 stringify", expression: "CryptoJS.enc.Utf8.stringify(CryptoJS.enc.Hex.parse('6162'))", want: "ab"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			script := postman.Script{
				Type: "text/javascript",
				Exec: []string{
					"pm.test('encodes', () => {",
					"    pm.expect(" + tc.expression + ").to.eql('" + tc.want + "');",
					"});",
				},
			}

			result := NewRuntime().ExecuteTestScript(script, &ExecutionContext{Response: &ResponseData{StatusCode: 200}})

			if len(result.Tests) != 1 || !result.Tests[0].Passed {
				t.Fatalf("expected %s to equal %q, got %+v", tc.expression, tc.want, result.Tests)
			}
		})
	}
}
