package postman

import (
	"encoding/json"
	"os"
	"path/filepath"
)

const configFileName = ".postoffice_collections.json"

type PersistenceConfig struct {
	CollectionPaths  []string `json:"collection_paths"`
	EnvironmentPaths []string `json:"environment_paths"`
}

func (p *Parser) SaveState() error {
	homeDir, err := os.UserHomeDir()
	if err != nil {
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
		return err
	}

	return os.WriteFile(configPath, data, 0644)
}

func (p *Parser) LoadState() error {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return err
	}

	configPath := filepath.Join(homeDir, configFileName)

	data, err := os.ReadFile(configPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	var config PersistenceConfig
	if err := json.Unmarshal(data, &config); err != nil {
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
