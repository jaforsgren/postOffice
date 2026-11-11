package dependency

import (
	"fmt"
	"os"
	"path/filepath"
	"postOffice/internal/logger"
	"strings"

	"gopkg.in/yaml.v3"
)

// fileSuffix replaces the collection file's extension to form the sidecar file name.
const fileSuffix = ".dependencies.yaml"

const filePermissions = 0644

// FilePath returns the dependency sidecar path for a collection file.
// E.g. "/path/to/MyCollection.json" → "/path/to/MyCollection.dependencies.yaml".
func FilePath(collectionPath string) string {
	return strings.TrimSuffix(collectionPath, filepath.Ext(collectionPath)) + fileSuffix
}

// Load reads a dependency sidecar file. A missing file yields an empty graph.
func Load(path string) (*Graph, error) {
	logger.LogFileOpen(path)
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return NewGraph(), nil
		}
		logger.LogError("LoadDependencies", path, err)
		return nil, fmt.Errorf("failed to read dependencies file: %w", err)
	}

	graph := NewGraph()
	if err := yaml.Unmarshal(data, graph); err != nil {
		logger.LogError("LoadDependencies", path, err)
		return nil, fmt.Errorf("failed to parse dependencies YAML %s: %w", path, err)
	}
	if graph.Requests == nil {
		graph.Requests = make(map[string]Node)
	}
	return graph, nil
}

// Save writes the graph to path as YAML.
func Save(graph *Graph, path string) error {
	data, err := yaml.Marshal(graph)
	if err != nil {
		return fmt.Errorf("failed to marshal dependencies: %w", err)
	}

	logger.LogFileWrite(path)
	if err := os.WriteFile(path, data, filePermissions); err != nil {
		logger.LogError("SaveDependencies", path, err)
		return fmt.Errorf("failed to write dependencies file: %w", err)
	}
	return nil
}
