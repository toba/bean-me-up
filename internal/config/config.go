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
	ConfigFileName = ".beans.clickup.yml"
	// BeansConfigFileName is the name of the beans CLI config file
	BeansConfigFileName = ".beans.yml"
)

// Config holds the bean-me-up configuration.
type Config struct {
	Beans BeansWrapper `yaml:"beans"`
}

// BeansWrapper wraps the ClickUp configuration under the beans key.
type BeansWrapper struct {
	ClickUp ClickUpConfig `yaml:"clickup"`
}

// ClickUpConfig holds ClickUp-specific settings.
type ClickUpConfig struct {
	ListID          string            `yaml:"list_id"`
	Assignee        *int              `yaml:"assignee,omitempty"`
	StatusMapping   map[string]string `yaml:"status_mapping,omitempty"`
	PriorityMapping map[string]int    `yaml:"priority_mapping,omitempty"`
	TypeMapping     map[string]int    `yaml:"type_mapping,omitempty"`
	CustomFields    *CustomFieldsMap  `yaml:"custom_fields,omitempty"`
	Users           map[string]int    `yaml:"users,omitempty"`
	SyncFilter      *SyncFilter       `yaml:"sync_filter,omitempty"`
}

// BeansConfig represents the beans CLI configuration.
type BeansConfig struct {
	Beans struct {
		Path string `yaml:"path"`
	} `yaml:"beans"`
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

	cfg := &Config{
		Beans: BeansWrapper{
			ClickUp: ClickUpConfig{
				StatusMapping:   DefaultStatusMapping,
				PriorityMapping: DefaultPriorityMapping,
			},
		},
	}
	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("parsing config: %w", err)
	}

	// Apply defaults for missing values
	if cfg.Beans.ClickUp.StatusMapping == nil {
		cfg.Beans.ClickUp.StatusMapping = DefaultStatusMapping
	}
	if cfg.Beans.ClickUp.PriorityMapping == nil {
		cfg.Beans.ClickUp.PriorityMapping = DefaultPriorityMapping
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

// LoadBeansPath loads the beans path from .beans.yml in the given directory.
// If not found, it searches upward. Returns the resolved absolute path.
func LoadBeansPath(startDir string) (string, error) {
	// Search upward for .beans.yml
	dir, err := filepath.Abs(startDir)
	if err != nil {
		return "", fmt.Errorf("finding beans config: %w", err)
	}

	var configPath string
	for {
		candidatePath := filepath.Join(dir, BeansConfigFileName)
		if _, err := os.Stat(candidatePath); err == nil {
			configPath = candidatePath
			break
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			// Reached filesystem root
			return "", fmt.Errorf("no %s found (searched from %s)", BeansConfigFileName, startDir)
		}
		dir = parent
	}

	data, err := os.ReadFile(configPath)
	if err != nil {
		return "", fmt.Errorf("reading beans config: %w", err)
	}

	var bc BeansConfig
	if err := yaml.Unmarshal(data, &bc); err != nil {
		return "", fmt.Errorf("parsing beans config: %w", err)
	}

	beansPath := bc.Beans.Path
	if beansPath == "" {
		beansPath = ".beans" // Default if not specified
	}

	// Resolve relative path against config file directory
	if !filepath.IsAbs(beansPath) {
		beansPath = filepath.Join(filepath.Dir(configPath), beansPath)
	}

	return beansPath, nil
}

// GetStatusMapping returns the effective status mapping.
func (c *Config) GetStatusMapping() map[string]string {
	if c.Beans.ClickUp.StatusMapping != nil {
		return c.Beans.ClickUp.StatusMapping
	}
	return DefaultStatusMapping
}

// GetPriorityMapping returns the effective priority mapping.
func (c *Config) GetPriorityMapping() map[string]int {
	if c.Beans.ClickUp.PriorityMapping != nil {
		return c.Beans.ClickUp.PriorityMapping
	}
	return DefaultPriorityMapping
}
