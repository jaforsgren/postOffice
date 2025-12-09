package postman

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type Parser struct {
	collections  map[string]*Collection
	pathMap      map[string]string
	environments map[string]*Environment
	envPathMap   map[string]string
}

func NewParser() *Parser {
	return &Parser{
		collections:  make(map[string]*Collection),
		pathMap:      make(map[string]string),
		environments: make(map[string]*Environment),
		envPathMap:   make(map[string]string),
	}
}

func (p *Parser) LoadCollection(path string) (*Collection, error) {
	expandedPath, err := expandPath(path)
	if err != nil {
		return nil, fmt.Errorf("failed to expand path: %w", err)
	}

	data, err := os.ReadFile(expandedPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read collection file: %w", err)
	}

	var collection Collection
	if err := json.Unmarshal(data, &collection); err != nil {
		return nil, fmt.Errorf("failed to parse collection: %w", err)
	}

	p.collections[collection.Info.Name] = &collection
	p.pathMap[collection.Info.Name] = expandedPath
	return &collection, nil
}

func expandPath(path string) (string, error) {
	path = strings.TrimSpace(path)
	if strings.HasPrefix(path, "~/") {
		homeDir, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		path = filepath.Join(homeDir, path[2:])
	}
	return path, nil
}

func (p *Parser) GetCollection(name string) (*Collection, bool) {
	collection, exists := p.collections[name]
	return collection, exists
}

func (p *Parser) ListCollections() []string {
	names := make([]string, 0, len(p.collections))
	for name := range p.collections {
		names = append(names, name)
	}
	return names
}

func (p *Parser) LoadEnvironment(path string) (*Environment, error) {
	expandedPath, err := expandPath(path)
	if err != nil {
		return nil, fmt.Errorf("failed to expand path: %w", err)
	}

	data, err := os.ReadFile(expandedPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read environment file: %w", err)
	}

	var environment Environment
	if err := json.Unmarshal(data, &environment); err != nil {
		return nil, fmt.Errorf("failed to parse environment: %w", err)
	}

	p.environments[environment.Name] = &environment
	p.envPathMap[environment.Name] = expandedPath
	return &environment, nil
}

func (p *Parser) GetEnvironment(name string) (*Environment, bool) {
	environment, exists := p.environments[name]
	return environment, exists
}

func (p *Parser) ListEnvironments() []string {
	names := make([]string, 0, len(p.environments))
	for name := range p.environments {
		names = append(names, name)
	}
	return names
}

func (p *Parser) SaveCollection(name string) error {
	collection, exists := p.collections[name]
	if !exists {
		return fmt.Errorf("collection not found: %s", name)
	}

	path, exists := p.pathMap[name]
	if !exists {
		return fmt.Errorf("collection path not found: %s", name)
	}

	data, err := json.MarshalIndent(collection, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal collection: %w", err)
	}

	tempPath := path + ".tmp"
	if err := os.WriteFile(tempPath, data, 0644); err != nil {
		return fmt.Errorf("failed to write temp file: %w", err)
	}

	if err := os.Rename(tempPath, path); err != nil {
		os.Remove(tempPath)
		return fmt.Errorf("failed to rename temp file: %w", err)
	}

	return nil
}

func (p *Parser) SaveEnvironment(name string) error {
	environment, exists := p.environments[name]
	if !exists {
		return fmt.Errorf("environment not found: %s", name)
	}

	path, exists := p.envPathMap[name]
	if !exists {
		return fmt.Errorf("environment path not found: %s", name)
	}

	data, err := json.MarshalIndent(environment, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal environment: %w", err)
	}

	tempPath := path + ".tmp"
	if err := os.WriteFile(tempPath, data, 0644); err != nil {
		return fmt.Errorf("failed to write temp file: %w", err)
	}

	if err := os.Rename(tempPath, path); err != nil {
		os.Remove(tempPath)
		return fmt.Errorf("failed to rename temp file: %w", err)
	}

	return nil
}
