package postman

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"postOffice/internal/logger"
	"time"
)

const savedResponsesFileName = ".postoffice_saved_responses.json"

type SavedResponse struct {
	SavedAt         time.Time           `json:"saved_at"`
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

func (p *Parser) SaveResponse(itemID string, resp SavedResponse) error {
	store, err := p.loadSavedResponseStore()
	if err != nil {
		return err
	}
	store[itemID] = append(store[itemID], resp)
	return p.writeSavedResponseStore(store)
}

func (p *Parser) GetSavedResponses(itemID string) ([]SavedResponse, error) {
	store, err := p.loadSavedResponseStore()
	if err != nil {
		return nil, err
	}
	return store[itemID], nil
}

func (p *Parser) DeleteSavedResponse(itemID string, index int) error {
	store, err := p.loadSavedResponseStore()
	if err != nil {
		return err
	}
	responses := store[itemID]
	if index < 0 || index >= len(responses) {
		return fmt.Errorf("index out of range: %d", index)
	}
	store[itemID] = append(responses[:index], responses[index+1:]...)
	if len(store[itemID]) == 0 {
		delete(store, itemID)
	}
	return p.writeSavedResponseStore(store)
}

func (p *Parser) loadSavedResponseStore() (map[string][]SavedResponse, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}
	path := filepath.Join(homeDir, savedResponsesFileName)
	logger.LogFileOpen(path)
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return make(map[string][]SavedResponse), nil
		}
		return nil, err
	}
	var store map[string][]SavedResponse
	if err := json.Unmarshal(data, &store); err != nil {
		return nil, err
	}
	return store, nil
}

func (p *Parser) writeSavedResponseStore(store map[string][]SavedResponse) error {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	path := filepath.Join(homeDir, savedResponsesFileName)
	data, err := json.MarshalIndent(store, "", "  ")
	if err != nil {
		return err
	}
	logger.LogFileWrite(path)
	return os.WriteFile(path, data, 0644)
}
