package dependency

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"postOffice/internal/logger"
	"strings"

	"gopkg.in/yaml.v3"
)

// fileSuffix replaces the collection file's extension to form the sidecar file name.
const fileSuffix = ".dependencies.yaml"

const (
	filePermissions = 0644
	yamlIndent      = 2
	requestsKey     = "requests"
	requiresKey     = "requires"
	producesKey     = "produces"
)

// FilePath returns the dependency sidecar path for a collection file.
// E.g. "/path/to/MyCollection.json" → "/path/to/MyCollection.dependencies.yaml".
func FilePath(collectionPath string) string {
	return strings.TrimSuffix(collectionPath, filepath.Ext(collectionPath)) + fileSuffix
}

// Load reads a dependency sidecar file. A missing or empty file yields an empty graph.
// Unknown keys are errors so a misspelt rule is not silently ignored.
func Load(path string) (*Graph, error) {
	data, err := readFile(path)
	if err != nil {
		return nil, err
	}

	graph := NewGraph()
	decoder := yaml.NewDecoder(bytes.NewReader(data))
	decoder.KnownFields(true)
	if err := decoder.Decode(graph); err != nil && !errors.Is(err, io.EOF) {
		logger.LogError("LoadDependencies", path, err)
		return nil, fmt.Errorf("failed to parse dependencies YAML %s: %w", path, err)
	}
	if graph.Requests == nil {
		graph.Requests = make(map[string]Node)
	}
	return graph, nil
}

// AddRequest declares requestPath with empty requires/produces lists unless it is
// already declared. It edits the YAML tree rather than the parsed graph so comments
// and the author's formatting of the other entries survive.
func AddRequest(path, requestPath string) error {
	data, err := readFile(path)
	if err != nil {
		return err
	}

	var document yaml.Node
	if err := yaml.Unmarshal(data, &document); err != nil {
		return fmt.Errorf("failed to parse dependencies YAML %s: %w", path, err)
	}
	if document.Kind == 0 {
		document = yaml.Node{Kind: yaml.DocumentNode, Content: []*yaml.Node{{Kind: yaml.MappingNode}}}
	}
	root := document.Content[0]
	if root.Kind != yaml.MappingNode {
		return fmt.Errorf("dependencies file %s must be a mapping with a %q key", path, requestsKey)
	}

	requests := mappingValue(root, requestsKey)
	if requests == nil {
		requests = &yaml.Node{Kind: yaml.MappingNode}
		root.Content = append(root.Content, scalarNode(requestsKey), requests)
	}
	if requests.Kind != yaml.MappingNode {
		// "requests:" with nothing under it parses as a null scalar.
		*requests = yaml.Node{Kind: yaml.MappingNode}
	}
	if mappingValue(requests, requestPath) != nil {
		return nil
	}
	requests.Content = append(requests.Content, scalarNode(requestPath), &yaml.Node{
		Kind: yaml.MappingNode,
		Content: []*yaml.Node{
			scalarNode(requiresKey), emptyFlowSequence(),
			scalarNode(producesKey), emptyFlowSequence(),
		},
	})

	var out bytes.Buffer
	encoder := yaml.NewEncoder(&out)
	encoder.SetIndent(yamlIndent)
	if err := encoder.Encode(&document); err != nil {
		return fmt.Errorf("failed to marshal dependencies: %w", err)
	}

	logger.LogFileWrite(path)
	if err := os.WriteFile(path, out.Bytes(), filePermissions); err != nil {
		logger.LogError("SaveDependencies", path, err)
		return fmt.Errorf("failed to write dependencies file: %w", err)
	}
	return nil
}

func readFile(path string) ([]byte, error) {
	logger.LogFileOpen(path)
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		logger.LogError("LoadDependencies", path, err)
		return nil, fmt.Errorf("failed to read dependencies file: %w", err)
	}
	return data, nil
}

func mappingValue(mapping *yaml.Node, key string) *yaml.Node {
	for i := 0; i+1 < len(mapping.Content); i += 2 {
		if mapping.Content[i].Value == key {
			return mapping.Content[i+1]
		}
	}
	return nil
}

func scalarNode(value string) *yaml.Node {
	return &yaml.Node{Kind: yaml.ScalarNode, Value: value}
}

func emptyFlowSequence() *yaml.Node {
	return &yaml.Node{Kind: yaml.SequenceNode, Style: yaml.FlowStyle}
}
