package postman

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"postOffice/internal/logger"
	"time"
)

const historyFileName = ".postoffice_history.json"

const maxHistoryPerItem = 50

type HistoryEntry struct {
	Timestamp       time.Time           `json:"timestamp"`
	RequestMethod   string              `json:"request_method"`
	RequestURL      string              `json:"request_url"`
	RequestHeaders  map[string]string   `json:"request_headers,omitempty"`
	RequestBody     string              `json:"request_body,omitempty"`
	StatusCode      int                 `json:"status_code"`
	Status          string              `json:"status"`
	ResponseHeaders map[string][]string `json:"response_headers,omitempty"`
	Body            string              `json:"body"`
	DurationMS      int64               `json:"duration_ms"`
}

func (p *Parser) AppendHistory(itemID string, entry HistoryEntry) error {
	store, err := p.loadHistoryStore()
	if err != nil {
		return err
	}
	entries := append(store[itemID], entry)
	if len(entries) > maxHistoryPerItem {
		entries = entries[len(entries)-maxHistoryPerItem:]
	}
	store[itemID] = entries
	return p.writeHistoryStore(store)
}

func (p *Parser) GetHistory(itemID string) ([]HistoryEntry, error) {
	store, err := p.loadHistoryStore()
	if err != nil {
		return nil, err
	}
	return store[itemID], nil
}

func (p *Parser) DeleteHistoryEntry(itemID string, index int) error {
	store, err := p.loadHistoryStore()
	if err != nil {
		return err
	}
	entries := store[itemID]
	if index < 0 || index >= len(entries) {
		return fmt.Errorf("index out of range: %d", index)
	}
	store[itemID] = append(entries[:index], entries[index+1:]...)
	if len(store[itemID]) == 0 {
		delete(store, itemID)
	}
	return p.writeHistoryStore(store)
}

func (p *Parser) loadHistoryStore() (map[string][]HistoryEntry, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}
	path := filepath.Join(homeDir, historyFileName)
	logger.LogFileOpen(path)
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return make(map[string][]HistoryEntry), nil
		}
		return nil, err
	}
	var store map[string][]HistoryEntry
	if err := json.Unmarshal(data, &store); err != nil {
		return nil, err
	}
	return store, nil
}

func (p *Parser) writeHistoryStore(store map[string][]HistoryEntry) error {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	path := filepath.Join(homeDir, historyFileName)
	data, err := json.MarshalIndent(store, "", "  ")
	if err != nil {
		return err
	}
	logger.LogFileWrite(path)
	return os.WriteFile(path, data, 0644)
}
