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
	CollectionPaths  []string `json:"collection_paths"`
	EnvironmentPaths []string `json:"environment_paths"`
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
		CollectionPaths:  collectionPaths,
		EnvironmentPaths: environmentPaths,
	}

	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		logger.LogError("SaveState", configPath, err)
		return err
	}

	logger.LogFileWrite(configPath)
	if err := os.WriteFile(configPath, data, 0644); err != nil {
		logger.LogError("SaveState", configPath, err)
		return err
	}

	return nil
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

	return nil
}

func (p *Parser) SaveSession(session *Session) error {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		logger.LogError("SaveSession", "UserHomeDir", err)
		return err
	}

	sessionPath := filepath.Join(homeDir, sessionFileName)

	data, err := json.MarshalIndent(session, "", "  ")
	if err != nil {
		logger.LogError("SaveSession", sessionPath, err)
		return err
	}

	logger.LogFileWrite(sessionPath)
	if err := os.WriteFile(sessionPath, data, 0644); err != nil {
		logger.LogError("SaveSession", sessionPath, err)
		return err
	}

	return nil
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
