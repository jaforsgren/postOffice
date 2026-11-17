package postman

import (
	"encoding/json"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"postOffice/internal/logger"
	"slices"
	"sort"
	"strings"
)

type Parser struct {
	collections  map[string]*Collection
	pathMap      map[string]string
	environments map[string]*Environment
	envPathMap   map[string]string

	// activeCollections and activeEnvironments track which loaded
	// collections/environments are marked active, in activation order
	// (earlier entries take precedence when merged).
	activeCollections  []string
	activeEnvironments []string
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
	expandedPath, err := ExpandPath(path)
	if err != nil {
		logger.LogError("LoadCollection", path, err)
		return nil, fmt.Errorf("failed to expand path: %w", err)
	}

	logger.LogFileOpen(expandedPath)
	data, err := os.ReadFile(expandedPath)
	if err != nil {
		logger.LogError("LoadCollection", expandedPath, err)
		return nil, fmt.Errorf("failed to read collection file: %w", err)
	}

	collection, err := ParseCollectionData(data)
	if err != nil {
		logger.LogError("LoadCollection", expandedPath, err)
		return nil, fmt.Errorf("failed to parse collection: %w", err)
	}

	p.collections[collection.Info.Name] = collection
	p.pathMap[collection.Info.Name] = expandedPath
	return collection, nil
}

// ParseCollectionData parses raw collection bytes, auto-detecting Postman vs OpenAPI format.
func ParseCollectionData(data []byte) (*Collection, error) {
	switch DetectFormat(data) {
	case FormatPostman:
		return parsePostmanCollection(data)
	case FormatOpenAPI:
		return parseOpenAPISpec(data)
	default:
		return nil, fmt.Errorf("unknown collection format")
	}
}

func parsePostmanCollection(data []byte) (*Collection, error) {
	var collection Collection
	if err := json.Unmarshal(data, &collection); err != nil {
		return nil, err
	}
	return &collection, nil
}

func parseOpenAPISpec(data []byte) (*Collection, error) {
	var spec OpenAPISpec
	if err := json.Unmarshal(data, &spec); err != nil {
		return nil, fmt.Errorf("failed to unmarshal OpenAPI spec: %w", err)
	}

	collection, err := ConvertOpenAPIToCollection(&spec)
	if err != nil {
		return nil, fmt.Errorf("failed to convert OpenAPI to collection: %w", err)
	}

	return collection, nil
}

// ExpandPath trims path, expands a leading "~/" to the home directory and
// makes it absolute, so a saved path still resolves from another directory.
func ExpandPath(path string) (string, error) {
	path = strings.TrimSpace(path)
	if strings.HasPrefix(path, "~/") {
		homeDir, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		path = filepath.Join(homeDir, path[2:])
	}
	return filepath.Abs(path)
}

func (p *Parser) GetCollection(name string) (*Collection, bool) {
	collection, exists := p.collections[name]
	return collection, exists
}

func (p *Parser) GetCollectionPath(name string) (string, bool) {
	path, exists := p.pathMap[name]
	return path, exists
}

func (p *Parser) GetEnvironmentPath(name string) (string, bool) {
	path, exists := p.envPathMap[name]
	return path, exists
}

func (p *Parser) ListCollections() []string {
	names := make([]string, 0, len(p.collections))
	for name := range p.collections {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func (p *Parser) LoadEnvironment(path string) (*Environment, error) {
	expandedPath, err := ExpandPath(path)
	if err != nil {
		logger.LogError("LoadEnvironment", path, err)
		return nil, fmt.Errorf("failed to expand path: %w", err)
	}

	logger.LogFileOpen(expandedPath)
	data, err := os.ReadFile(expandedPath)
	if err != nil {
		logger.LogError("LoadEnvironment", expandedPath, err)
		return nil, fmt.Errorf("failed to read environment file: %w", err)
	}

	var environment Environment
	if err := json.Unmarshal(data, &environment); err != nil {
		logger.LogError("LoadEnvironment", expandedPath, err)
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
	sort.Strings(names)
	return names
}

// ToggleActiveCollection adds name to the active set if absent, or removes it
// if present. Order reflects activation order, which MergedCollection uses as
// precedence (earlier group folders keep their variables on conflict).
func (p *Parser) ToggleActiveCollection(name string) {
	p.activeCollections = toggleName(p.activeCollections, name)
}

// ToggleActiveEnvironment adds name to the active set if absent, or removes
// it if present. Order reflects activation order, which MergedEnvironment
// uses as precedence (earlier environments win on key conflicts).
func (p *Parser) ToggleActiveEnvironment(name string) {
	p.activeEnvironments = toggleName(p.activeEnvironments, name)
}

// ActivateOnlyEnvironment replaces the active set with name alone.
func (p *Parser) ActivateOnlyEnvironment(name string) {
	p.activeEnvironments = []string{name}
}

func (p *Parser) IsCollectionActive(name string) bool {
	return containsName(p.activeCollections, name)
}

func (p *Parser) IsEnvironmentActive(name string) bool {
	return containsName(p.activeEnvironments, name)
}

func (p *Parser) ActiveCollections() []string {
	return slices.Clone(p.activeCollections)
}

func (p *Parser) ActiveEnvironments() []string {
	return slices.Clone(p.activeEnvironments)
}

func (p *Parser) ActiveCollectionCount() int {
	return len(p.activeCollections)
}

func (p *Parser) ActiveEnvironmentCount() int {
	return len(p.activeEnvironments)
}

func toggleName(list []string, name string) []string {
	if i := slices.Index(list, name); i >= 0 {
		return slices.Delete(list, i, i+1)
	}
	return append(list, name)
}

func containsName(list []string, name string) bool {
	return slices.Contains(list, name)
}

// MergedCollection combines every active collection into a single browsing
// tree, nesting each collection's items under a top-level group folder named
// after it so items never collide across collections. It returns nil if no
// collections are active. The result's Info.Name is not a real collection
// name, so it never resolves through pathMap/collections lookups: edits made
// in the tree reach the collections through SyncMergedCollection.
func (p *Parser) MergedCollection() *Collection {
	var items []Item
	var variables []Variable
	var events []Event
	var names []string
	seenVars := make(map[string]bool)

	for _, name := range p.activeCollections {
		coll, exists := p.collections[name]
		if !exists {
			continue
		}
		names = append(names, name)
		items = append(items, Item{Name: name, Items: coll.Items})

		for _, v := range coll.Variables {
			if !seenVars[v.Key] {
				variables = append(variables, v)
				seenVars[v.Key] = true
			}
		}
		events = mergeEvents(events, coll.Events)
	}

	if len(names) == 0 {
		return nil
	}

	return &Collection{
		Info:      Info{Name: strings.Join(names, " + ")},
		Items:     items,
		Variables: variables,
		Events:    events,
	}
}

// SyncMergedCollection writes each group folder of merged, as built by
// MergedCollection, back to the collection it groups. Items below a group
// folder are shared with that collection, but the group's own item list is a
// copy, so items added to or removed from a collection's top level in the
// merged tree only reach the collection through this.
func (p *Parser) SyncMergedCollection(merged *Collection) {
	for _, group := range merged.Items {
		if coll, exists := p.collections[group.Name]; exists && containsName(p.activeCollections, group.Name) {
			coll.Items = group.Items
		}
	}
}

// SyncMergedEnvironment writes changes made to a MergedEnvironment copy (by
// scripts, say) back to the active environments it combines, and returns the
// names of those that changed, in precedence order. A changed key goes to the
// first active environment defining it, the one it resolves from; a new key
// to the first active environment. A key missing from merged is removed from
// every active environment, since any one left would resolve again.
func (p *Parser) SyncMergedEnvironment(merged *Environment) []string {
	var active []*Environment
	for _, name := range p.activeEnvironments {
		if environment, exists := p.environments[name]; exists {
			active = append(active, environment)
		}
	}
	if len(active) == 0 {
		return nil
	}

	changed := make(map[string]bool)
	mergedKeys := make(map[string]bool, len(merged.Values))
	for _, variable := range merged.Values {
		mergedKeys[variable.Key] = true
		owner, index := firstDefining(active, variable.Key)
		if owner == nil {
			active[0].Values = append(active[0].Values, variable)
			changed[active[0].Name] = true
			continue
		}
		if !sameEnvVariable(owner.Values[index], variable) {
			owner.Values[index] = variable
			changed[owner.Name] = true
		}
	}
	for _, environment := range active {
		count := len(environment.Values)
		environment.Values = slices.DeleteFunc(environment.Values, func(v EnvVariable) bool { return !mergedKeys[v.Key] })
		if len(environment.Values) != count {
			changed[environment.Name] = true
		}
	}

	var names []string
	for _, environment := range active {
		if changed[environment.Name] {
			names = append(names, environment.Name)
		}
	}
	return names
}

func firstDefining(environments []*Environment, key string) (*Environment, int) {
	for _, environment := range environments {
		if index := slices.IndexFunc(environment.Values, func(v EnvVariable) bool { return v.Key == key }); index >= 0 {
			return environment, index
		}
	}
	return nil, -1
}

func sameEnvVariable(a, b EnvVariable) bool {
	return a.Value == b.Value && a.Enabled == b.Enabled && a.Type == b.Type && slices.Equal(a.Choices, b.Choices) && maps.Equal(a.ChoiceNotes, b.ChoiceNotes)
}

// MergedEnvironment combines every active environment into one, in
// activation order (earlier environments take precedence on key conflicts).
// It returns nil if no environments are active. The result's Name is not a
// real environment name, so it never resolves through envPathMap — callers
// use that to detect and block edits against the merged view.
func (p *Parser) MergedEnvironment() *Environment {
	var merged *Environment
	var names []string

	for _, name := range p.activeEnvironments {
		env, exists := p.environments[name]
		if !exists {
			continue
		}
		names = append(names, name)

		if merged == nil {
			seed := *env
			seed.Values = slices.Clone(env.Values)
			merged = &seed
			continue
		}
		merged = MergeEnvironments(merged, env, MergeKeepOriginal)
	}

	if merged == nil {
		return nil
	}

	merged.ID = "merged"
	merged.Name = strings.Join(names, " + ")
	return merged
}

func (p *Parser) SaveCollection(name string) error {
	collection, exists := p.collections[name]
	if !exists {
		err := fmt.Errorf("collection not found: %s", name)
		logger.LogError("SaveCollection", name, err)
		return err
	}

	path, exists := p.pathMap[name]
	if !exists {
		err := fmt.Errorf("collection path not found: %s", name)
		logger.LogError("SaveCollection", name, err)
		return err
	}

	return saveToFile(collection, path, "SaveCollection")
}

func (p *Parser) SaveEnvironment(name string) error {
	environment, exists := p.environments[name]
	if !exists {
		err := fmt.Errorf("environment not found: %s", name)
		logger.LogError("SaveEnvironment", name, err)
		return err
	}

	path, exists := p.envPathMap[name]
	if !exists {
		err := fmt.Errorf("environment path not found: %s", name)
		logger.LogError("SaveEnvironment", name, err)
		return err
	}

	return saveToFile(environment, path, "SaveEnvironment")
}

// saveToFile writes data to path atomically: marshal to a uniquely-named
// temp file in the same directory, then rename over path. The unique name
// (rather than a fixed path+".tmp") matters because postOffice can have
// multiple sessions running concurrently against the same shared config
// files (e.g. ~/.postoffice_collections.json) — a fixed temp name would let
// two sessions saving at once collide mid-write and produce a corrupt or
// unexpectedly-swapped file. Rename itself is atomic, so a reader never sees
// a partially-written file.
//
// If path is a symlink (e.g. config kept in a dotfiles repo), the write goes
// to the link target — renaming over the link itself would replace it with a
// regular file and silently detach it from the dotfiles copy.
func saveToFile(data any, path string, logLabel string) error {
	bytes, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		logger.LogError(logLabel, path, err)
		return fmt.Errorf("failed to marshal %s: %w", logLabel, err)
	}

	if resolved, err := filepath.EvalSymlinks(path); err == nil {
		path = resolved
	}

	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, filepath.Base(path)+".*.tmp")
	if err != nil {
		logger.LogError(logLabel, dir, err)
		return fmt.Errorf("failed to create temp file: %w", err)
	}
	tempPath := tmp.Name()

	if _, err := tmp.Write(bytes); err != nil {
		tmp.Close()
		os.Remove(tempPath)
		logger.LogError(logLabel, tempPath, err)
		return fmt.Errorf("failed to write temp file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tempPath)
		logger.LogError(logLabel, tempPath, err)
		return fmt.Errorf("failed to close temp file: %w", err)
	}

	logger.LogFileWrite(tempPath)
	logger.LogFileWrite(path)
	if err := os.Rename(tempPath, path); err != nil {
		os.Remove(tempPath)
		logger.LogError(logLabel, path, err)
		return fmt.Errorf("failed to rename temp file: %w", err)
	}

	return nil
}
