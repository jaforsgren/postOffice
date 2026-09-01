package postman

import (
	"testing"
	"time"
)

func newTestParserWithIsolatedHome(t *testing.T) *Parser {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	return NewParser()
}

func TestAppendAndGetHistory(t *testing.T) {
	p := newTestParserWithIsolatedHome(t)

	if err := p.AppendHistory("col/req", HistoryEntry{Timestamp: time.Now(), Status: "200 OK", StatusCode: 200}); err != nil {
		t.Fatalf("AppendHistory: %v", err)
	}
	if err := p.AppendHistory("col/req", HistoryEntry{Timestamp: time.Now(), Status: "404 Not Found", StatusCode: 404}); err != nil {
		t.Fatalf("AppendHistory: %v", err)
	}

	entries, err := p.GetHistory("col/req")
	if err != nil {
		t.Fatalf("GetHistory: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(entries))
	}
	if entries[0].StatusCode != 200 || entries[1].StatusCode != 404 {
		t.Fatalf("unexpected entry order: %+v", entries)
	}

	other, err := p.GetHistory("col/other")
	if err != nil {
		t.Fatalf("GetHistory (unknown item): %v", err)
	}
	if len(other) != 0 {
		t.Fatalf("expected no entries for unknown item, got %d", len(other))
	}
}

func TestAppendHistoryTrimsToMaxPerItem(t *testing.T) {
	p := newTestParserWithIsolatedHome(t)

	for i := 0; i < maxHistoryPerItem+10; i++ {
		if err := p.AppendHistory("col/req", HistoryEntry{StatusCode: i}); err != nil {
			t.Fatalf("AppendHistory: %v", err)
		}
	}

	entries, err := p.GetHistory("col/req")
	if err != nil {
		t.Fatalf("GetHistory: %v", err)
	}
	if len(entries) != maxHistoryPerItem {
		t.Fatalf("expected trimmed length %d, got %d", maxHistoryPerItem, len(entries))
	}
	if entries[0].StatusCode != 10 {
		t.Fatalf("expected oldest entries dropped, first StatusCode = %d", entries[0].StatusCode)
	}
	if entries[len(entries)-1].StatusCode != maxHistoryPerItem+9 {
		t.Fatalf("expected newest entry kept, last StatusCode = %d", entries[len(entries)-1].StatusCode)
	}
}

func TestDeleteHistoryEntry(t *testing.T) {
	p := newTestParserWithIsolatedHome(t)

	if err := p.AppendHistory("col/req", HistoryEntry{StatusCode: 200}); err != nil {
		t.Fatalf("AppendHistory: %v", err)
	}
	if err := p.AppendHistory("col/req", HistoryEntry{StatusCode: 500}); err != nil {
		t.Fatalf("AppendHistory: %v", err)
	}

	if err := p.DeleteHistoryEntry("col/req", 0); err != nil {
		t.Fatalf("DeleteHistoryEntry: %v", err)
	}

	entries, err := p.GetHistory("col/req")
	if err != nil {
		t.Fatalf("GetHistory: %v", err)
	}
	if len(entries) != 1 || entries[0].StatusCode != 500 {
		t.Fatalf("unexpected entries after delete: %+v", entries)
	}

	if err := p.DeleteHistoryEntry("col/req", 5); err == nil {
		t.Fatal("expected error for out-of-range index")
	}
}
