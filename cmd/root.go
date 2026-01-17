// Package cmd implements the bean-me-up CLI commands.
package cmd

import (
	"fmt"
	"os"

	"github.com/STR-Consulting/bean-me-up/internal/config"
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
	Use:   "bean-me-up",
	Short: "Sync beans to ClickUp",
	Long: `bean-me-up syncs beans (from the beans CLI) to ClickUp tasks.

It works as a companion tool to the standard beans CLI, storing sync
state in .beans/.sync.json without modifying bean files.

Configuration is stored in .bean-me-up.yml in your project directory.`,
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		// Skip config loading for help commands
		if cmd.Name() == "help" || cmd.Name() == "completion" {
			return nil
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
			configDir = cwd
		} else {
			cfg, configDir, err = config.LoadFromDirectory(cwd)
			if err != nil {
				return fmt.Errorf("loading config: %w", err)
			}
		}

		// Override beans path if specified
		if beansPath != "" {
			cfg.BeansPath = beansPath
		}

		return nil
	},
}

// Execute runs the root command.
func Execute() error {
	return rootCmd.Execute()
}

func init() {
	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file (default: .bean-me-up.yml)")
	rootCmd.PersistentFlags().StringVar(&beansPath, "beans-path", "", "path to beans directory (default: from config)")
	rootCmd.PersistentFlags().BoolVar(&jsonOut, "json", false, "output as JSON")
}

// getBeansPath returns the resolved beans path.
func getBeansPath() string {
	return cfg.ResolveBeansPath(configDir)
}

// getClickUpToken returns the ClickUp API token from environment.
func getClickUpToken() (string, error) {
	token := os.Getenv("CLICKUP_TOKEN")
	if token == "" {
		return "", fmt.Errorf("CLICKUP_TOKEN environment variable is not set")
	}
	return token, nil
}
