package protocol

import (
	"context"
	"postOffice/internal/http"
	"postOffice/internal/postman"
	"postOffice/internal/script"
	"testing"
)

func TestBuiltinRegistry_BashRunsPreRequestAndTestScripts(t *testing.T) {
	item := &postman.Item{
		Name: "greet",
		Request: &postman.Request{
			Method: postman.MethodBash,
			URL:    postman.URL{Raw: postman.SchemeBash},
			Body:   &postman.Body{Mode: postman.BodyModeRaw, Raw: `echo "hello {{name}}"`},
		},
		Events: []postman.Event{
			scriptEvent(script.PhasePreRequest, "pm.collectionVariables.set('name', 'bash');"),
			scriptEvent(script.PhaseTest,
				"pm.test('greets', () => pm.expect(pm.response.text()).to.include('hello bash'));",
				"pm.test('exits 0', () => pm.response.to.have.status(200));",
			),
		},
	}
	bash, err := NewBuiltinRegistry(http.NewExecutor(), nil).For(item)
	if err != nil {
		t.Fatalf("For() error = %v", err)
	}
	scope := &script.RequestScope{Collection: &postman.Collection{}, Item: item}

	resp, result := bash.Execute(context.Background(), item.Request, scope, nil)

	if resp.Error != nil || resp.Body != "hello bash\n" {
		t.Fatalf("response = %+v, want the script output with the pre-request variable", resp)
	}
	if result == nil || len(result.Errors) > 0 || len(result.Tests) != 2 {
		t.Fatalf("result = %+v, want two tests and no errors", result)
	}
	for _, test := range result.Tests {
		if !test.Passed {
			t.Errorf("test %q failed: %s", test.Name, test.Error)
		}
	}
}
