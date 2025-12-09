package postman

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type Parser struct {
	collections map[string]*Collection
	pathMap     map[string]string
}

func NewParser() *Parser {
	return &Parser{
		collections: make(map[string]*Collection),
		pathMap:     make(map[string]string),
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
