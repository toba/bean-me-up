// Package config handles configuration for bean-me-up.
package config

import (
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/toba/bean-me-up/internal/beans"
	"gopkg.in/yaml.v3"
)

const (
	// LegacyConfigFileName is the name of the legacy standalone config file
	LegacyConfigFileName = ".beans.clickup.yml"
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

	SyncFilter      *SyncFilter       `yaml:"sync_filter,omitempty"`
}

// BeansConfig represents the beans CLI configuration.
type BeansConfig struct {
	Beans struct {
		Path string `yaml:"path"`
	} `yaml:"beans"`
}

// beansYMLExtensions is used to parse the extensions section from .beans.yml.
type beansYMLExtensions struct {
	Extensions struct {
		ClickUp ClickUpConfig `yaml:"clickup"`
	} `yaml:"extensions"`
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

// FindConfig searches upward from the given directory for a legacy config file.
// Returns the absolute path to the config file, or empty string if not found.
func FindConfig(startDir string) (string, error) {
	dir, err := filepath.Abs(startDir)
	if err != nil {
		return "", err
	}

	for {
		configPath := filepath.Join(dir, LegacyConfigFileName)
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

// Load reads configuration from a legacy .beans.clickup.yml file path.
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

	applyDefaults(cfg)
	return cfg, nil
}

// LoadFromBeansYML reads ClickUp config from the extensions section of .beans.yml.
func LoadFromBeansYML(beansYMLPath string) (*Config, error) {
	data, err := os.ReadFile(beansYMLPath)
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", beansYMLPath, err)
	}

	var ext beansYMLExtensions
	if err := yaml.Unmarshal(data, &ext); err != nil {
		return nil, fmt.Errorf("parsing %s: %w", beansYMLPath, err)
	}

	// Check if extensions.clickup is actually configured (list_id is the minimum)
	if ext.Extensions.ClickUp.ListID == "" {
		return nil, fmt.Errorf("no extensions.clickup section found in %s", beansYMLPath)
	}

	cfg := &Config{
		Beans: BeansWrapper{
			ClickUp: ext.Extensions.ClickUp,
		},
	}

	applyDefaults(cfg)
	return cfg, nil
}

// applyDefaults fills in default values and validates type mappings.
func applyDefaults(cfg *Config) {
	if cfg.Beans.ClickUp.StatusMapping == nil {
		cfg.Beans.ClickUp.StatusMapping = DefaultStatusMapping
	}
	if cfg.Beans.ClickUp.PriorityMapping == nil {
		cfg.Beans.ClickUp.PriorityMapping = DefaultPriorityMapping
	}

	// Validate type mapping keys are standard bean types
	if cfg.Beans.ClickUp.TypeMapping != nil {
		validMapping := make(map[string]int)
		for beanType, clickupTypeID := range cfg.Beans.ClickUp.TypeMapping {
			if beans.IsStandardType(beanType) {
				validMapping[beanType] = clickupTypeID
			} else {
				log.Printf("Warning: ignoring invalid bean type %q in type_mapping (valid types: %v)", beanType, beans.StandardTypes)
			}
		}
		cfg.Beans.ClickUp.TypeMapping = validMapping
	}
}

// LoadFromDirectory finds and loads config by searching for .beans.yml extensions
// first, then falling back to legacy .beans.clickup.yml.
func LoadFromDirectory(startDir string) (*Config, string, error) {
	dir, err := filepath.Abs(startDir)
	if err != nil {
		return nil, "", err
	}

	// First, try .beans.yml extensions section
	beansYMLPath := findFileUpward(dir, BeansConfigFileName)
	if beansYMLPath != "" {
		cfg, err := LoadFromBeansYML(beansYMLPath)
		if err == nil {
			return cfg, filepath.Dir(beansYMLPath), nil
		}
		// extensions.clickup not found in .beans.yml, fall through to legacy
	}

	// Fall back to legacy .beans.clickup.yml
	legacyPath := findFileUpward(dir, LegacyConfigFileName)
	if legacyPath == "" {
		return nil, "", fmt.Errorf("no ClickUp config found (searched for extensions.clickup in %s and %s from %s)",
			BeansConfigFileName, LegacyConfigFileName, startDir)
	}

	cfg, err := Load(legacyPath)
	if err != nil {
		return nil, "", err
	}

	return cfg, filepath.Dir(legacyPath), nil
}

// findFileUpward searches upward from dir for a file with the given name.
// Returns the absolute path if found, or empty string if not.
func findFileUpward(dir, filename string) string {
	for {
		candidate := filepath.Join(dir, filename)
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			return ""
		}
		dir = parent
	}
}

// LoadBeansPath loads the beans path from .beans.yml in the given directory.
// If not found, it searches upward. Returns the resolved absolute path.
func LoadBeansPath(startDir string) (string, error) {
	// Search upward for .beans.yml
	dir, err := filepath.Abs(startDir)
	if err != nil {
		return "", fmt.Errorf("finding beans config: %w", err)
	}

	configPath := findFileUpward(dir, BeansConfigFileName)
	if configPath == "" {
		return "", fmt.Errorf("no %s found (searched from %s)", BeansConfigFileName, startDir)
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
