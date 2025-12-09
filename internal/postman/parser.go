package postman

import (
	"encoding/json"
	"fmt"
	"os"
)

type Parser struct {
	collections map[string]*Collection
}

func NewParser() *Parser {
	return &Parser{
		collections: make(map[string]*Collection),
	}
}

func (p *Parser) LoadCollection(path string) (*Collection, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read collection file: %w", err)
	}

	var collection Collection
	if err := json.Unmarshal(data, &collection); err != nil {
		return nil, fmt.Errorf("failed to parse collection: %w", err)
	}

	p.collections[collection.Info.Name] = &collection
	return &collection, nil
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
