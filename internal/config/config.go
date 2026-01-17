// Package config handles configuration for bean-me-up.
package config

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

const (
	// ConfigFileName is the name of the config file
	ConfigFileName = ".bean-me-up.yml"
	// DefaultBeansPath is the default beans directory
	DefaultBeansPath = ".beans"
)

// Config holds the bean-me-up configuration.
type Config struct {
	BeansPath string        `yaml:"beans_path,omitempty"`
	ClickUp   ClickUpConfig `yaml:"clickup"`
}

// ClickUpConfig holds ClickUp-specific settings.
type ClickUpConfig struct {
	ListID          string            `yaml:"list_id"`
	Assignee        *int              `yaml:"assignee,omitempty"`
	StatusMapping   map[string]string `yaml:"status_mapping,omitempty"`
	PriorityMapping map[string]int    `yaml:"priority_mapping,omitempty"`
	CustomFields    *CustomFieldsMap  `yaml:"custom_fields,omitempty"`
	Users           map[string]int    `yaml:"users,omitempty"`
	SyncFilter      *SyncFilter       `yaml:"sync_filter,omitempty"`
}

// CustomFieldsMap maps bean fields to ClickUp custom field UUIDs.
type CustomFieldsMap struct {
	BeanID    string `yaml:"bean_id,omitempty"`
	CreatedAt string `yaml:"created_at,omitempty"`
	UpdatedAt string `yaml:"updated_at,omitempty"`
}

// SyncFilter defines which beans to sync.
type SyncFilter struct {
	ExcludeStatus []string `yaml:"exclude_status,omitempty"`
}

// DefaultStatusMapping provides standard bean→ClickUp status mapping.
var DefaultStatusMapping = map[string]string{
	"draft":       "backlog",
	"todo":        "to do",
	"in-progress": "in progress",
	"completed":   "complete",
	"scrapped":    "closed",
}

// DefaultPriorityMapping provides standard bean→ClickUp priority mapping.
// ClickUp priorities: 1=Urgent, 2=High, 3=Normal, 4=Low
var DefaultPriorityMapping = map[string]int{
	"critical": 1,
	"high":     2,
	"normal":   3,
	"low":      4,
	"deferred": 4,
}

// Default returns a Config with default values.
func Default() *Config {
	return &Config{
		BeansPath: DefaultBeansPath,
		ClickUp: ClickUpConfig{
			StatusMapping:   DefaultStatusMapping,
			PriorityMapping: DefaultPriorityMapping,
		},
	}
}

// FindConfig searches upward from the given directory for a config file.
// Returns the absolute path to the config file, or empty string if not found.
func FindConfig(startDir string) (string, error) {
	dir, err := filepath.Abs(startDir)
	if err != nil {
		return "", err
	}

	for {
		configPath := filepath.Join(dir, ConfigFileName)
		if _, err := os.Stat(configPath); err == nil {
			return configPath, nil
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			// Reached filesystem root
			return "", nil
		}
		dir = parent
	}
}

// Load reads configuration from the given config file path.
func Load(configPath string) (*Config, error) {
	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil, fmt.Errorf("reading config: %w", err)
	}

	cfg := Default()
	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("parsing config: %w", err)
	}

	// Apply defaults for missing values
	if cfg.BeansPath == "" {
		cfg.BeansPath = DefaultBeansPath
	}
	if cfg.ClickUp.StatusMapping == nil {
		cfg.ClickUp.StatusMapping = DefaultStatusMapping
	}
	if cfg.ClickUp.PriorityMapping == nil {
		cfg.ClickUp.PriorityMapping = DefaultPriorityMapping
	}

	return cfg, nil
}

// LoadFromDirectory finds and loads the config file by searching upward.
func LoadFromDirectory(startDir string) (*Config, string, error) {
	configPath, err := FindConfig(startDir)
	if err != nil {
		return nil, "", err
	}

	if configPath == "" {
		return nil, "", fmt.Errorf("no %s found (searched from %s)", ConfigFileName, startDir)
	}

	cfg, err := Load(configPath)
	if err != nil {
		return nil, "", err
	}

	return cfg, filepath.Dir(configPath), nil
}

// ResolveBeansPath returns the absolute path to the beans directory.
func (c *Config) ResolveBeansPath(configDir string) string {
	if filepath.IsAbs(c.BeansPath) {
		return c.BeansPath
	}
	return filepath.Join(configDir, c.BeansPath)
}

// GetStatusMapping returns the effective status mapping.
func (c *Config) GetStatusMapping() map[string]string {
	if c.ClickUp.StatusMapping != nil {
		return c.ClickUp.StatusMapping
	}
	return DefaultStatusMapping
}

// GetPriorityMapping returns the effective priority mapping.
func (c *Config) GetPriorityMapping() map[string]int {
	if c.ClickUp.PriorityMapping != nil {
		return c.ClickUp.PriorityMapping
	}
	return DefaultPriorityMapping
}
