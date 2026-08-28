package postman

import (
	"encoding/json"
	"os"
	"path/filepath"
	"postOffice/internal/logger"
)

const configFileName = ".postoffice_collections.json"
const sessionFileName = ".postoffice_session.json"

type PersistenceConfig struct {
	CollectionPaths        []string `json:"collection_paths"`
	EnvironmentPaths       []string `json:"environment_paths"`
	ActiveCollectionNames  []string `json:"active_collection_names,omitempty"`
	ActiveEnvironmentNames []string `json:"active_environment_names,omitempty"`
}

// mergeUniqueStrings unions a and b, preserving a's order first, then any of
// b's entries not already present in a.
func mergeUniqueStrings(a, b []string) []string {
	seen := make(map[string]bool, len(a)+len(b))
	result := make([]string, 0, len(a)+len(b))

	for _, v := range a {
		if !seen[v] {
			seen[v] = true
			result = append(result, v)
		}
	}
	for _, v := range b {
		if !seen[v] {
			seen[v] = true
			result = append(result, v)
		}
	}

	return result
}

type Session struct {
	CollectionName  string   `json:"collection_name"`
	EnvironmentName string   `json:"environment_name"`
	Mode            int      `json:"mode"`
	Breadcrumb      []string `json:"breadcrumb"`
	Cursor          int      `json:"cursor"`
}

func (p *Parser) SaveState() error {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		logger.LogError("SaveState", "UserHomeDir", err)
		return err
	}

	configPath := filepath.Join(homeDir, configFileName)

	// Read whatever is currently on disk so a second, concurrently running
	// session doesn't clobber collections/environments a different session
	// loaded — paths only ever grow (there's no unload), so it's always safe
	// to union them rather than overwrite. Missing/corrupt file is fine; it
	// just means there's nothing to merge with yet.
	var onDisk PersistenceConfig
	if data, err := os.ReadFile(configPath); err == nil {
		_ = json.Unmarshal(data, &onDisk)
	}

	collectionPaths := make([]string, 0)
	for name := range p.collections {
		if path, exists := p.pathMap[name]; exists {
			collectionPaths = append(collectionPaths, path)
		}
	}

	environmentPaths := make([]string, 0)
	for name := range p.environments {
		if path, exists := p.envPathMap[name]; exists {
			environmentPaths = append(environmentPaths, path)
		}
	}

	config := PersistenceConfig{
		CollectionPaths:        mergeUniqueStrings(onDisk.CollectionPaths, collectionPaths),
		EnvironmentPaths:       mergeUniqueStrings(onDisk.EnvironmentPaths, environmentPaths),
		ActiveCollectionNames:  p.ActiveCollections(),
		ActiveEnvironmentNames: p.ActiveEnvironments(),
	}

	return saveToFile(config, configPath, "SaveState")
}

func (p *Parser) LoadState() error {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		logger.LogError("LoadState", "UserHomeDir", err)
		return err
	}

	configPath := filepath.Join(homeDir, configFileName)

	logger.LogFileOpen(configPath)
	data, err := os.ReadFile(configPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		logger.LogError("LoadState", configPath, err)
		return err
	}

	var config PersistenceConfig
	if err := json.Unmarshal(data, &config); err != nil {
		logger.LogError("LoadState", configPath, err)
		return err
	}

	for _, path := range config.CollectionPaths {
		_, _ = p.LoadCollection(path)
	}

	for _, path := range config.EnvironmentPaths {
		_, _ = p.LoadEnvironment(path)
	}

	for _, name := range config.ActiveCollectionNames {
		if _, exists := p.collections[name]; exists {
			p.activeCollections = append(p.activeCollections, name)
		}
	}

	for _, name := range config.ActiveEnvironmentNames {
		if _, exists := p.environments[name]; exists {
			p.activeEnvironments = append(p.activeEnvironments, name)
		}
	}

	return nil
}

func (p *Parser) SaveSession(session *Session) error {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		logger.LogError("SaveSession", "UserHomeDir", err)
		return err
	}

	sessionPath := filepath.Join(homeDir, sessionFileName)

	return saveToFile(session, sessionPath, "SaveSession")
}

func (p *Parser) LoadSession() (*Session, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		logger.LogError("LoadSession", "UserHomeDir", err)
		return nil, err
	}

	sessionPath := filepath.Join(homeDir, sessionFileName)

	logger.LogFileOpen(sessionPath)
	data, err := os.ReadFile(sessionPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		logger.LogError("LoadSession", sessionPath, err)
		return nil, err
	}

	var session Session
	if err := json.Unmarshal(data, &session); err != nil {
		logger.LogError("LoadSession", sessionPath, err)
		return nil, err
	}

	return &session, nil
}
