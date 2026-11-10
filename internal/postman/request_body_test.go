package postman

import (
	"encoding/json"
	"errors"
	"reflect"
	"testing"
)

func TestFormParamFilePaths(t *testing.T) {
	cases := map[string]struct {
		src  string
		want []string
	}{
		"one path":       {`"/tmp/a.png"`, []string{"/tmp/a.png"}},
		"several paths":  {`["/tmp/a.png","/tmp/b.png"]`, []string{"/tmp/a.png", "/tmp/b.png"}},
		"no file picked": {`""`, nil},
		"null":           {`null`, nil},
		"absent":         {``, nil},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			got, err := FormParam{Key: "file", Src: json.RawMessage(tc.src)}.FilePaths()
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("got %v, want %v", got, tc.want)
			}
		})
	}
}

func TestFormParamFilePaths_InvalidSource(t *testing.T) {
	_, err := FormParam{Key: "file", Src: json.RawMessage(`{"path":"/tmp/a.png"}`)}.FilePaths()

	if !errors.Is(err, ErrInvalidFormFileSource) {
		t.Errorf("expected ErrInvalidFormFileSource, got %v", err)
	}
}

func TestRequestClone_IsDeepAndKeepsUnmodelledFields(t *testing.T) {
	var original Request
	source := `{"method":"POST","auth":{"type":"noauth"},"header":[{"key":"a","value":"1"}],` +
		`"body":{"mode":"formdata","formdata":[{"key":"file","type":"file","src":"/tmp/a.png"}]},` +
		`"url":{"raw":"https://example.com/api","host":["example","com"],"path":["api"],"query":[{"key":"q"}]}}`
	if err := json.Unmarshal([]byte(source), &original); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	copied := original.Clone()
	copied.Header[0].Value = "changed"
	copied.URL.Path[0] = "changed"
	copied.Body.FormData[0].Key = "changed"
	copied.Body.FormData[0].Src[1] = 'X'

	saved, err := json.Marshal(&original)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if got, want := jsonValue(t, saved), jsonValue(t, []byte(source)); !reflect.DeepEqual(got, want) {
		t.Errorf("changing the clone changed the original: %s", saved)
	}
	clonedJSON, _ := json.Marshal(copied)
	if clone := jsonValue(t, clonedJSON).(map[string]any); clone["auth"] == nil {
		t.Errorf("expected the clone to keep auth, got %s", clonedJSON)
	}
}

func TestRequestClone_Nil(t *testing.T) {
	var request *Request

	if request.Clone() != nil {
		t.Error("expected nil when cloning nil")
	}
}
