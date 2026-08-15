// Package alias persists named shortcuts for headless commands (e.g. "postOffice -m ...")
// so they can be replayed later via `postOffice alias run <name>` without retyping
// file paths and flags.
package alias

import (
	"encoding/json"
	"os"
	"path/filepath"
)

const configFileName = ".postoffice_aliases.json"

// Alias is a saved invocation of a headless command: the command name (as
// registered in main's headlessCommands map) plus the args to replay.
type Alias struct {
	Command string   `json:"command"`
	Args    []string `json:"args"`
}

type Config struct {
	Aliases map[string]Alias `json:"aliases"`
}

func configPath() (string, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(homeDir, configFileName), nil
}

// Load reads the alias config file, returning an empty config if it doesn't exist yet.
func Load() (*Config, error) {
	path, err := configPath()
	if err != nil {
		return nil, err
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return &Config{Aliases: make(map[string]Alias)}, nil
		}
		return nil, err
	}

	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}
	if cfg.Aliases == nil {
		cfg.Aliases = make(map[string]Alias)
	}
	return &cfg, nil
}

func (c *Config) Save() error {
	path, err := configPath()
	if err != nil {
		return err
	}

	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(path, data, 0644)
}

func (c *Config) Set(name string, a Alias) {
	c.Aliases[name] = a
}

func (c *Config) Get(name string) (Alias, bool) {
	a, exists := c.Aliases[name]
	return a, exists
}

func (c *Config) Remove(name string) bool {
	if _, exists := c.Aliases[name]; !exists {
		return false
	}
	delete(c.Aliases, name)
	return true
}
