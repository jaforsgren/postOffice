package history

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"postOffice/internal/logger"
)

const (
	fileName = ".postoffice_history.json"

	// MaxEntries caps the history; the oldest entries are dropped first.
	MaxEntries = 200

	// Entries contain resolved secrets (auth headers, connection strings), so unlike
	// the other ~/.postoffice_* files the history is readable by the owner only.
	fileMode os.FileMode = 0o600
)

// Store keeps a chronological, size-capped request history.
// A Store with an empty path keeps history in memory only.
type Store struct {
	path    string
	entries []Entry
}

func NewStore(path string) *Store {
	return &Store{path: path}
}

// DefaultPath returns the history file location in the user's home directory.
func DefaultPath() (string, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		logger.LogError("HistoryDefaultPath", "UserHomeDir", err)
		return "", fmt.Errorf("resolve home directory for request history: %w", err)
	}
	return filepath.Join(homeDir, fileName), nil
}

// Load replaces the in-memory history with the persisted one. A missing file is not an error.
func (s *Store) Load() error {
	if s.path == "" {
		return nil
	}

	logger.LogFileOpen(s.path)
	data, err := os.ReadFile(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		logger.LogError("LoadHistory", s.path, err)
		return fmt.Errorf("read request history %s: %w", s.path, err)
	}

	var entries []Entry
	if err := json.Unmarshal(data, &entries); err != nil {
		logger.LogError("LoadHistory", s.path, err)
		return fmt.Errorf("parse request history %s: %w", s.path, err)
	}

	s.entries = trimToMax(entries)
	return nil
}

// Add appends entry, drops the oldest entries beyond MaxEntries and persists the result.
// The entry is kept in memory even when persisting fails.
func (s *Store) Add(entry Entry) error {
	s.entries = trimToMax(append(s.entries, entry))
	return s.save()
}

// Entries returns a copy of the history, newest first.
func (s *Store) Entries() []Entry {
	newestFirst := make([]Entry, len(s.entries))
	for i, entry := range s.entries {
		newestFirst[len(s.entries)-1-i] = entry
	}
	return newestFirst
}

func (s *Store) Len() int {
	return len(s.entries)
}

func (s *Store) save() error {
	if s.path == "" {
		return nil
	}

	data, err := json.MarshalIndent(s.entries, "", "  ")
	if err != nil {
		logger.LogError("SaveHistory", s.path, err)
		return fmt.Errorf("encode request history: %w", err)
	}

	logger.LogFileWrite(s.path)
	if err := os.WriteFile(s.path, data, fileMode); err != nil {
		logger.LogError("SaveHistory", s.path, err)
		return fmt.Errorf("write request history %s: %w", s.path, err)
	}
	return nil
}

func trimToMax(entries []Entry) []Entry {
	if len(entries) <= MaxEntries {
		return entries
	}
	return append([]Entry(nil), entries[len(entries)-MaxEntries:]...)
}
