package postman

import (
	"encoding/json"
	"os"
	"path/filepath"
	"postOffice/internal/logger"
)

const configFileName = ".postoffice_collections.json"
const sessionFileName = ".postoffice_session.json"
const favoritesFileName = ".postoffice_favorites.json"

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
	CollectionName  string `json:"collection_name"`
	EnvironmentName string `json:"environment_name"`
	// CombinedView reopens the combined view of the active collections
	// instead of CollectionName.
	CombinedView bool     `json:"combined_view,omitempty"`
	Mode         int      `json:"mode"`
	Breadcrumb   []string `json:"breadcrumb"`
	Cursor       int      `json:"cursor"`
	// Watches are the PEEK requests the TUI was watching, resumed on startup.
	Watches []WatchedRequest `json:"watches,omitempty"`
	// CLI is the headless commands' own selection, kept apart from the TUI's
	// so neither overwrites the other.
	CLI *CLISelection `json:"cli,omitempty"`
}

// WatchedRequest names a watched PEEK request by its collection, its folder
// path within that collection and its name.
type WatchedRequest struct {
	Collection string   `json:"collection"`
	FolderPath []string `json:"folder_path,omitempty"`
	Request    string   `json:"request"`
}

// CLISelection is the collection, environment and workflow headless commands
// use when none is given, as file paths.
type CLISelection struct {
	CollectionPath  string `json:"collection_path,omitempty"`
	EnvironmentPath string `json:"environment_path,omitempty"`
	WorkflowPath    string `json:"workflow_path,omitempty"`
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

// SaveSession saves the TUI's session, keeping the stored CLI selection.
func (p *Parser) SaveSession(session *Session) error {
	var toSave Session
	if session != nil {
		toSave = *session
	}
	toSave.CLI = nil
	if stored, err := p.LoadSession(); err == nil && stored != nil {
		toSave.CLI = stored.CLI
	}
	return saveSessionFile(&toSave, "SaveSession")
}

// LoadCLISelection returns the stored CLI selection, empty when none is
// stored or the session file can't be read.
func (p *Parser) LoadCLISelection() *CLISelection {
	session, err := p.LoadSession()
	if err != nil || session == nil || session.CLI == nil {
		return &CLISelection{}
	}
	return session.CLI
}

// SaveCLISelection saves the CLI selection, keeping the stored TUI session.
func (p *Parser) SaveCLISelection(selection *CLISelection) error {
	session, err := p.LoadSession()
	if err != nil || session == nil {
		session = &Session{}
	}
	session.CLI = selection
	return saveSessionFile(session, "SaveCLISelection")
}

func saveSessionFile(session *Session, logLabel string) error {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		logger.LogError(logLabel, "UserHomeDir", err)
		return err
	}
	return saveToFile(session, filepath.Join(homeDir, sessionFileName), logLabel)
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

// FavoritesConfig persists the set of favorited requests, identified by the
// same "collection/folder/.../name" IDs used elsewhere in the TUI.
type FavoritesConfig struct {
	ItemIDs []string `json:"item_ids"`
}

// SaveFavorites writes the full set of favorited item IDs to disk, overwriting
// whatever was there before — the TUI always calls this with its complete
// in-memory set, so there's nothing to merge.
func SaveFavorites(itemIDs []string) error {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		logger.LogError("SaveFavorites", "UserHomeDir", err)
		return err
	}

	favoritesPath := filepath.Join(homeDir, favoritesFileName)
	return saveToFile(FavoritesConfig{ItemIDs: itemIDs}, favoritesPath, "SaveFavorites")
}

// LoadFavorites reads the favorited item IDs from disk. A missing file is not
// an error — it just means nothing has been favorited yet.
func LoadFavorites() ([]string, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		logger.LogError("LoadFavorites", "UserHomeDir", err)
		return nil, err
	}

	favoritesPath := filepath.Join(homeDir, favoritesFileName)

	logger.LogFileOpen(favoritesPath)
	data, err := os.ReadFile(favoritesPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		logger.LogError("LoadFavorites", favoritesPath, err)
		return nil, err
	}

	var config FavoritesConfig
	if err := json.Unmarshal(data, &config); err != nil {
		logger.LogError("LoadFavorites", favoritesPath, err)
		return nil, err
	}

	return config.ItemIDs, nil
}
