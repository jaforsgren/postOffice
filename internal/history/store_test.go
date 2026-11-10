package history

import (
	"fmt"
	"os"
	"path/filepath"
	"postOffice/internal/postman"
	"reflect"
	"testing"
	"time"
)

func newEntry(name string) Entry {
	return Entry{
		ItemID:          "Collection/" + name,
		ItemName:        name,
		Protocol:        ProtocolHTTP,
		EnvironmentName: "dev",
		Snapshot: postman.SavedResponse{
			SavedAt:        time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC),
			RequestMethod:  "POST",
			RequestURL:     "https://api.example.com/" + name,
			RequestHeaders: map[string]string{"X-B": "2", "Authorization": "Bearer t"},
			RequestBody:    `{"a":1}`,
			StatusCode:     200,
			Status:         "200 OK",
			Body:           "hello",
			DurationMS:     42,
		},
	}
}

func TestStore_EntriesNewestFirst(t *testing.T) {
	store := NewStore("")
	for _, name := range []string{"first", "second", "third"} {
		if err := store.Add(newEntry(name)); err != nil {
			t.Fatalf("Add() error = %v", err)
		}
	}

	entries := store.Entries()
	got := []string{entries[0].ItemName, entries[1].ItemName, entries[2].ItemName}
	want := []string{"third", "second", "first"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Entries() order = %v, want %v", got, want)
	}
}

func TestStore_CapsAtMaxEntries(t *testing.T) {
	store := NewStore("")
	for i := 0; i < MaxEntries+5; i++ {
		_ = store.Add(newEntry(fmt.Sprintf("req-%d", i)))
	}

	if store.Len() != MaxEntries {
		t.Fatalf("Len() = %d, want %d", store.Len(), MaxEntries)
	}
	entries := store.Entries()
	if newest := entries[0].ItemName; newest != fmt.Sprintf("req-%d", MaxEntries+4) {
		t.Errorf("newest = %s", newest)
	}
	if oldest := entries[len(entries)-1].ItemName; oldest != "req-5" {
		t.Errorf("oldest = %s, want req-5", oldest)
	}
}

func TestStore_PersistsAndReloads(t *testing.T) {
	path := filepath.Join(t.TempDir(), "history.json")
	store := NewStore(path)
	_ = store.Add(newEntry("one"))
	_ = store.Add(newEntry("two"))

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("history file not written: %v", err)
	}
	if info.Mode().Perm() != fileMode {
		t.Errorf("file mode = %v, want %v", info.Mode().Perm(), fileMode)
	}

	reloaded := NewStore(path)
	if err := reloaded.Load(); err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if !reflect.DeepEqual(reloaded.Entries(), store.Entries()) {
		t.Errorf("reloaded entries differ:\n got %+v\nwant %+v", reloaded.Entries(), store.Entries())
	}
}

func TestStore_LoadMissingFileIsEmpty(t *testing.T) {
	store := NewStore(filepath.Join(t.TempDir(), "missing.json"))
	if err := store.Load(); err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if store.Len() != 0 {
		t.Errorf("Len() = %d, want 0", store.Len())
	}
}

func TestStore_LoadInvalidJSON(t *testing.T) {
	path := filepath.Join(t.TempDir(), "history.json")
	if err := os.WriteFile(path, []byte("{not json"), fileMode); err != nil {
		t.Fatal(err)
	}
	if err := NewStore(path).Load(); err == nil {
		t.Error("Load() error = nil, want parse error")
	}
}

func TestStore_LoadTrimsOversizedFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "history.json")
	writer := NewStore(path)
	writer.entries = make([]Entry, MaxEntries+10)
	if err := writer.save(); err != nil {
		t.Fatal(err)
	}

	store := NewStore(path)
	if err := store.Load(); err != nil {
		t.Fatal(err)
	}
	if store.Len() != MaxEntries {
		t.Errorf("Len() = %d, want %d", store.Len(), MaxEntries)
	}
}

func TestStore_AddKeepsEntryWhenWriteFails(t *testing.T) {
	store := NewStore(filepath.Join(t.TempDir(), "no-such-dir", "history.json"))
	if err := store.Add(newEntry("one")); err == nil {
		t.Error("Add() error = nil, want write error")
	}
	if store.Len() != 1 {
		t.Errorf("Len() = %d, want 1", store.Len())
	}
}

func TestEntry_Request(t *testing.T) {
	req := newEntry("orders").Request()

	if req.Method != "POST" || req.URL.Raw != "https://api.example.com/orders" {
		t.Errorf("method/url = %s %s", req.Method, req.URL.Raw)
	}
	wantHeaders := []postman.Header{{Key: "Authorization", Value: "Bearer t"}, {Key: "X-B", Value: "2"}}
	if !reflect.DeepEqual(req.Header, wantHeaders) {
		t.Errorf("headers = %v, want %v", req.Header, wantHeaders)
	}
	if req.Body == nil || req.Body.Raw != `{"a":1}` || req.Body.Mode != postman.BodyModeRaw {
		t.Errorf("body = %+v", req.Body)
	}
}

func TestEntry_RequestWithoutBody(t *testing.T) {
	entry := newEntry("ping")
	entry.Snapshot.RequestBody = ""
	if body := entry.Request().Body; body != nil {
		t.Errorf("body = %+v, want nil", body)
	}
}

func TestEntry_ResponseSize(t *testing.T) {
	if size := newEntry("x").ResponseSize(); size != len("hello") {
		t.Errorf("ResponseSize() = %d, want %d", size, len("hello"))
	}
}
