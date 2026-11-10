package http

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"postOffice/internal/postman"
)

const pngFileContent = "\x89PNG fake"

type receivedForm struct {
	contentType string
	fields      map[string][]string
	files       map[string][]receivedFile
	parseErr    error
}

type receivedFile struct {
	name        string
	contentType string
	content     string
}

func formServer(t *testing.T, received *receivedForm) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		received.contentType = r.Header.Get("Content-Type")
		reader, err := r.MultipartReader()
		if err != nil {
			received.parseErr = err
			return
		}
		received.fields = map[string][]string{}
		received.files = map[string][]receivedFile{}
		for {
			part, err := reader.NextPart()
			if err == io.EOF {
				return
			}
			if err != nil {
				received.parseErr = err
				return
			}
			content, _ := io.ReadAll(part)
			if part.FileName() == "" {
				received.fields[part.FormName()] = append(received.fields[part.FormName()], string(content))
				continue
			}
			received.files[part.FormName()] = append(received.files[part.FormName()], receivedFile{
				name:        part.FileName(),
				contentType: part.Header.Get("Content-Type"),
				content:     string(content),
			})
		}
	}))
	t.Cleanup(server.Close)
	return server
}

func writeTempFile(t *testing.T, name, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}
	return path
}

func fileSrc(paths ...string) json.RawMessage {
	if len(paths) == 1 {
		src, _ := json.Marshal(paths[0])
		return src
	}
	src, _ := json.Marshal(paths)
	return src
}

func formRequest(url string, params ...postman.FormParam) *postman.Request {
	return &postman.Request{
		Method: "POST",
		URL:    postman.URL{Raw: url},
		Header: []postman.Header{{Key: "Content-Type", Value: "multipart/form-data"}},
		Body:   &postman.Body{Mode: postman.BodyModeFormData, FormData: params},
	}
}

func TestExecute_FormDataSendsFieldsAndFiles(t *testing.T) {
	var received receivedForm
	server := formServer(t, &received)
	picture := writeTempFile(t, "picture.png", pngFileContent)
	notes := writeTempFile(t, "notes.txt", "hello")
	variables := []postman.VariableSource{{Key: "scope", Value: "Customer"}}

	resp, _ := NewExecutor().Execute(formRequest(server.URL,
		postman.FormParam{Key: "scope", Value: "{{scope}}", Type: "text"},
		postman.FormParam{Key: "skipped", Value: "x", Type: "text", Disabled: true},
		postman.FormParam{Key: "file", Type: postman.FormParamTypeFile, Src: fileSrc(picture)},
		postman.FormParam{Key: "extra", Type: postman.FormParamTypeFile, Src: fileSrc(picture, notes), ContentType: "application/x-custom"},
	), nil, variables)

	if resp.Error != nil || received.parseErr != nil {
		t.Fatalf("request error %v, server parse error %v (Content-Type %q)", resp.Error, received.parseErr, received.contentType)
	}
	if got := received.fields["scope"]; len(got) != 1 || got[0] != "Customer" {
		t.Errorf("scope field = %v, want [Customer]", got)
	}
	if _, sent := received.fields["skipped"]; sent {
		t.Error("expected a disabled field not to be sent")
	}
	if got := received.files["file"]; len(got) != 1 || got[0] != (receivedFile{name: "picture.png", contentType: "image/png", content: pngFileContent}) {
		t.Errorf("file part = %+v", got)
	}
	if got := received.files["extra"]; len(got) != 2 || got[1].name != "notes.txt" || got[1].contentType != "application/x-custom" {
		t.Errorf("extra parts = %+v", got)
	}
}

func TestExecute_FormDataWithoutFieldsIsStillMultipart(t *testing.T) {
	var received receivedForm
	server := formServer(t, &received)

	resp, _ := NewExecutor().Execute(formRequest(server.URL), nil, nil)

	if resp.Error != nil || received.parseErr != nil {
		t.Fatalf("request error %v, server parse error %v (Content-Type %q)", resp.Error, received.parseErr, received.contentType)
	}
	if len(received.fields) != 0 || len(received.files) != 0 {
		t.Errorf("expected an empty form, got fields %v files %v", received.fields, received.files)
	}
}

func TestExecute_FormDataFileFieldWithoutAFileSendsNoPart(t *testing.T) {
	var received receivedForm
	server := formServer(t, &received)

	resp, _ := NewExecutor().Execute(formRequest(server.URL, postman.FormParam{Key: "file", Type: postman.FormParamTypeFile}), nil, nil)

	if resp.Error != nil || received.parseErr != nil {
		t.Fatalf("request error %v, server parse error %v", resp.Error, received.parseErr)
	}
	if len(received.files) != 0 {
		t.Errorf("expected no file part, got %v", received.files)
	}
}

func TestExecute_FormDataUnreadableFileIsAnError(t *testing.T) {
	var hits int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { hits++ }))
	defer server.Close()
	missing := filepath.Join(t.TempDir(), "missing.png")

	resp, _ := NewExecutor().Execute(formRequest(server.URL, postman.FormParam{Key: "file", Type: postman.FormParamTypeFile, Src: fileSrc(missing)}), nil, nil)

	if !errors.Is(resp.Error, ErrFormFileUnreadable) {
		t.Errorf("expected ErrFormFileUnreadable, got %v", resp.Error)
	}
	if hits != 0 {
		t.Error("expected nothing to be sent")
	}
}
