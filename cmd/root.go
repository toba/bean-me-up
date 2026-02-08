// Package cmd implements the bean-me-up CLI commands.
package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/toba/bean-me-up/internal/config"
	"github.com/spf13/cobra"
)

var (
	// Global flags
	cfgFile   string
	beansPath string
	jsonOut   bool

	// Loaded configuration
	cfg       *config.Config
	configDir string
)

var rootCmd = &cobra.Command{
	Use:   "beanup",
	Short: "Sync beans to ClickUp",
	Long: `beanup syncs beans (from the beans CLI) to ClickUp tasks.

It works as a companion tool to the standard beans CLI, storing sync
state in bean extension metadata.

Configuration is stored in the extensions.clickup section of .beans.yml,
or in a legacy .beans.clickup.yml file.`,
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		// Skip config loading for help commands and init
		if cmd.Name() == "help" || cmd.Name() == "completion" || cmd.Name() == "init" || cmd.Name() == "migrate" {
			return nil
		}

		// Check if beans CLI is installed
		if !checkBeansInstalled() {
			fmt.Fprintln(os.Stderr, "Warning: beans CLI not found in PATH")
		}

		// Load configuration
		cwd, err := os.Getwd()
		if err != nil {
			return fmt.Errorf("getting working directory: %w", err)
		}

		if cfgFile != "" {
			cfg, err = config.Load(cfgFile)
			if err != nil {
				return fmt.Errorf("loading config: %w", err)
			}
			configDir = filepath.Dir(cfgFile)
		} else {
			cfg, configDir, err = config.LoadFromDirectory(cwd)
			if err != nil {
				return fmt.Errorf("loading config: %w", err)
			}
		}

		return nil
	},
}

// Execute runs the root command.
func Execute() error {
	return rootCmd.Execute()
}

func init() {
	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "Path to legacy .beans.clickup.yml config file")
	rootCmd.PersistentFlags().StringVar(&beansPath, "beans-path", "", "path to beans directory (default: from .beans.yml)")
	rootCmd.PersistentFlags().BoolVar(&jsonOut, "json", false, "output as JSON")
}

// checkBeansInstalled returns true if the beans CLI is installed.
func checkBeansInstalled() bool {
	_, err := exec.LookPath("beans")
	return err == nil
}

// getBeansPath returns the resolved beans path.
// Priority: 1) --beans-path flag, 2) beans.path from .beans.yml
func getBeansPath() string {
	if beansPath != "" {
		return beansPath
	}

	// Load from .beans.yml
	path, err := config.LoadBeansPath(configDir)
	if err != nil {
		// Fall back to default if .beans.yml not found
		return ".beans"
	}
	return path
}

// getClickUpToken returns the ClickUp API token from environment.
func getClickUpToken() (string, error) {
	token := os.Getenv("CLICKUP_TOKEN")
	if token == "" {
		return "", fmt.Errorf("CLICKUP_TOKEN environment variable is not set")
	}
	return token, nil
}

// outputJSON writes a value as indented JSON to stdout.
func outputJSON(v interface{}) error {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}

// requireListID returns an error if list_id is not configured.
func requireListID() error {
	if cfg.Beans.ClickUp.ListID == "" {
		return fmt.Errorf("ClickUp list_id is required in .beans.yml extensions.clickup or .beans.clickup.yml")
	}
	return nil
}
