package postman

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

// A Postman export trimmed to the keys postOffice does not model, at every level.
const collectionWithUnmodelledFields = `{
  "info": {"_postman_id": "abc", "name": "SAPI", "description": "d", "schema": "s"},
  "auth": {"type": "bearer", "bearer": [{"key": "token", "value": "{{sejfaToken}}", "type": "string"}]},
  "item": [{
    "name": "folder",
    "auth": {"type": "noauth"},
    "item": [{
      "name": "upload",
      "request": {
        "auth": {"type": "noauth"},
        "method": "POST",
        "header": [{"key": "authKey", "value": "anything", "disabled": true}],
        "body": {
          "mode": "formdata",
          "formdata": [{"key": "file", "type": "file", "src": "/tmp/a.png", "fileNotInWorkingDirectoryWarning": "w"}],
          "options": {"raw": {"language": "json"}}
        },
        "url": {"raw": "{{base}}/x?a=1", "host": ["{{base}}"], "path": ["x"], "query": [{"key": "a", "value": "1"}], "variable": [{"key": "id", "value": "1"}]},
        "description": "uploads"
      },
      "event": [{"listen": "test", "script": {"id": "s1", "type": "text/javascript", "exec": ["pm.test()"], "packages": {}}}],
      "response": [{"name": "saved", "code": 202}]
    }]
  }],
  "variable": [{"id": "v1", "key": "base", "value": "http://localhost", "type": "string"}]
}`

func jsonValue(t *testing.T, data []byte) any {
	t.Helper()
	var value any
	if err := json.Unmarshal(data, &value); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, data)
	}
	return value
}

func TestCollectionRoundTrip_KeepsUnmodelledFields(t *testing.T) {
	var collection Collection
	if err := json.Unmarshal([]byte(collectionWithUnmodelledFields), &collection); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	saved, err := json.MarshalIndent(&collection, "", "  ")
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	want := jsonValue(t, []byte(collectionWithUnmodelledFields))
	if got := jsonValue(t, saved); !reflect.DeepEqual(got, want) {
		t.Errorf("round trip changed the collection:\n%s", saved)
	}
}

func TestCollectionRoundTrip_EditedModelledFieldsWin(t *testing.T) {
	var collection Collection
	if err := json.Unmarshal([]byte(collectionWithUnmodelledFields), &collection); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	collection.Items[0].Items[0].Request.Method = "PUT"

	saved, err := json.Marshal(&collection)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	if !strings.Contains(string(saved), `"method":"PUT"`) || strings.Contains(string(saved), `"method":"POST"`) {
		t.Errorf("expected only the edited method, got %s", saved)
	}
}

func TestEnvironmentRoundTrip_KeepsUnmodelledFields(t *testing.T) {
	const environment = `{"id":"e","name":"Big tusks","values":[{"key":"k","value":"v","enabled":true,"type":"secret","description":"d"}],"_postman_variable_scope":"environment"}`
	var parsed Environment
	if err := json.Unmarshal([]byte(environment), &parsed); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	saved, err := json.Marshal(&parsed)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	if got, want := jsonValue(t, saved), jsonValue(t, []byte(environment)); !reflect.DeepEqual(got, want) {
		t.Errorf("round trip changed the environment: %s", saved)
	}
}

func TestRoundTrip_UnmodelledFieldsFollowModelledOnes(t *testing.T) {
	var header Header
	if err := json.Unmarshal([]byte(`{"disabled":true,"value":"v","key":"k"}`), &header); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	saved, err := json.Marshal(header)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	if want := `{"key":"k","value":"v","disabled":true}`; string(saved) != want {
		t.Errorf("got %s, want %s", saved, want)
	}
}

func TestEncodeWithUnknown_OnlyUnmodelledFields(t *testing.T) {
	saved, err := encodeWithUnknown(struct{}{}, `{"auth":{"type":"noauth"}}`)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}

	if want := `{"auth":{"type":"noauth"}}`; string(saved) != want {
		t.Errorf("got %s, want %s", saved, want)
	}
}
