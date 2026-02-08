package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/toba/bean-me-up/internal/beans"
	"github.com/toba/bean-me-up/internal/config"
	"github.com/toba/bean-me-up/internal/syncstate"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

var (
	migrateDryRun         bool
	migrateDeleteSyncFile bool
)

var migrateCmd = &cobra.Command{
	Use:   "migrate",
	Short: "Migrate sync state from .sync.json to bean extension metadata",
	Long: `Migrates legacy beanup configuration and sync state:

1. Moves ClickUp config from .beans.clickup.yml into the extensions.clickup
   section of .beans.yml (and deletes the legacy file).

2. Migrates sync state from .beans/.sync.json into each bean's extension
   metadata.

Use --dry-run to preview the migration without making changes.
Use --delete-sync-file to also remove .sync.json after a successful migration.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		bp := getBeansPath()
		if bp == "" {
			bp = ".beans"
		}

		// Migrate .beans.clickup.yml → .beans.yml extensions.clickup
		if err := migrateConfig(bp, migrateDryRun); err != nil {
			return err
		}

		// Load legacy sync state
		syncFilePath := filepath.Join(bp, syncstate.SyncFileName)
		if _, err := os.Stat(syncFilePath); os.IsNotExist(err) {
			fmt.Println("No .sync.json found — nothing to migrate.")
			return nil
		}

		store, err := syncstate.Load(bp)
		if err != nil {
			return fmt.Errorf("loading sync state: %w", err)
		}

		allBeans := store.GetAllBeans()
		if len(allBeans) == 0 {
			fmt.Println("No sync entries found in .sync.json — nothing to migrate.")
			return nil
		}

		// List existing beans so we can skip stale IDs from .sync.json
		beansClient := beans.NewClient(bp)
		existingBeans, err := beansClient.List()
		if err != nil {
			return fmt.Errorf("listing beans: %w", err)
		}
		existingIDs := make(map[string]bool, len(existingBeans))
		for _, b := range existingBeans {
			existingIDs[b.ID] = true
		}

		// Build batch operations
		var ops []beans.ExtensionDataOp
		var skipped int
		for beanID, beanSync := range allBeans {
			if beanSync.ClickUp == nil || beanSync.ClickUp.TaskID == "" {
				continue
			}
			if !existingIDs[beanID] {
				skipped++
				continue
			}

			data := map[string]any{
				beans.ExtKeyTaskID: beanSync.ClickUp.TaskID,
			}
			if beanSync.ClickUp.SyncedAt != nil {
				data[beans.ExtKeySyncedAt] = beanSync.ClickUp.SyncedAt.Format(time.RFC3339)
			}

			ops = append(ops, beans.ExtensionDataOp{
				ID:   beanID,
				Name: beans.PluginClickUp,
				Data:   data,
			})
		}

		if len(ops) == 0 {
			fmt.Println("No linked beans to migrate.")
			return nil
		}
		if skipped > 0 {
			fmt.Printf("Skipping %d bean(s) no longer present.\n", skipped)
		}

		if migrateDryRun {
			fmt.Printf("Would migrate %d bean(s):\n", len(ops))
			for _, op := range ops {
				taskID := op.Data[beans.ExtKeyTaskID]
				fmt.Printf("  %s → clickup.task_id=%v\n", op.ID, taskID)
			}
			if migrateDeleteSyncFile {
				fmt.Printf("\nWould delete %s\n", syncFilePath)
			}
			return nil
		}

		// Execute batch migration
		fmt.Printf("Migrating %d bean(s)...\n", len(ops))

		if err := beansClient.SetExtensionDataBatch(ops); err != nil {
			return fmt.Errorf("writing extension data: %w", err)
		}

		fmt.Printf("Migrated %d bean(s) to extension metadata.\n", len(ops))

		// Optionally delete the sync file
		if migrateDeleteSyncFile {
			if err := os.Remove(syncFilePath); err != nil {
				return fmt.Errorf("deleting %s: %w", syncstate.SyncFileName, err)
			}
			fmt.Printf("Deleted %s\n", syncFilePath)
		} else {
			fmt.Printf("\nTo remove the legacy file: beanup migrate --delete-sync-file\n")
		}

		return nil
	},
}

// migrateConfig moves ClickUp configuration from .beans.clickup.yml into .beans.yml
// under extensions.clickup, then deletes the legacy file.
func migrateConfig(beansPath string, dryRun bool) error {
	// Find the directory containing .beans.yml by walking up from the beans path
	beansDir, err := filepath.Abs(beansPath)
	if err != nil {
		return fmt.Errorf("resolving beans path: %w", err)
	}
	// The config files live in the project root, which is the parent of .beans/
	projectDir := filepath.Dir(beansDir)

	legacyPath := filepath.Join(projectDir, config.LegacyConfigFileName)
	beansYMLPath := filepath.Join(projectDir, config.BeansConfigFileName)

	if _, err := os.Stat(legacyPath); os.IsNotExist(err) {
		return nil // No legacy config to migrate
	}

	// Load the legacy config
	legacyCfg, err := config.Load(legacyPath)
	if err != nil {
		return fmt.Errorf("loading %s: %w", config.LegacyConfigFileName, err)
	}

	if dryRun {
		fmt.Printf("Would merge %s into %s (extensions.clickup)\n", config.LegacyConfigFileName, config.BeansConfigFileName)
		fmt.Printf("Would delete %s\n\n", legacyPath)
		return nil
	}

	// Read existing .beans.yml as a generic map to preserve all fields
	beansYML := make(map[string]any)
	if data, err := os.ReadFile(beansYMLPath); err == nil {
		if err := yaml.Unmarshal(data, &beansYML); err != nil {
			return fmt.Errorf("parsing %s: %w", config.BeansConfigFileName, err)
		}
	}

	// Build the clickup extension config as a map for clean YAML output
	clickupMap := make(map[string]any)
	cu := legacyCfg.Beans.ClickUp

	clickupMap["list_id"] = cu.ListID
	if cu.Assignee != nil {
		clickupMap["assignee"] = *cu.Assignee
	}
	if cu.StatusMapping != nil {
		clickupMap["status_mapping"] = cu.StatusMapping
	}
	if cu.PriorityMapping != nil {
		clickupMap["priority_mapping"] = cu.PriorityMapping
	}
	if cu.TypeMapping != nil {
		clickupMap["type_mapping"] = cu.TypeMapping
	}
	if cu.CustomFields != nil {
		cf := make(map[string]any)
		if cu.CustomFields.BeanID != "" {
			cf["bean_id"] = cu.CustomFields.BeanID
		}
		if cu.CustomFields.CreatedAt != "" {
			cf["created_at"] = cu.CustomFields.CreatedAt
		}
		if cu.CustomFields.UpdatedAt != "" {
			cf["updated_at"] = cu.CustomFields.UpdatedAt
		}
		if len(cf) > 0 {
			clickupMap["custom_fields"] = cf
		}
	}
	if cu.SyncFilter != nil {
		sf := make(map[string]any)
		if len(cu.SyncFilter.ExcludeStatus) > 0 {
			sf["exclude_status"] = cu.SyncFilter.ExcludeStatus
		}
		if len(sf) > 0 {
			clickupMap["sync_filter"] = sf
		}
	}

	// Merge into extensions.clickup
	extensions, _ := beansYML["extensions"].(map[string]any)
	if extensions == nil {
		extensions = make(map[string]any)
	}
	extensions["clickup"] = clickupMap
	beansYML["extensions"] = extensions

	// Write updated .beans.yml
	out, err := yaml.Marshal(beansYML)
	if err != nil {
		return fmt.Errorf("marshaling %s: %w", config.BeansConfigFileName, err)
	}
	if err := os.WriteFile(beansYMLPath, out, 0644); err != nil {
		return fmt.Errorf("writing %s: %w", config.BeansConfigFileName, err)
	}

	// Delete legacy file
	if err := os.Remove(legacyPath); err != nil {
		return fmt.Errorf("deleting %s: %w", config.LegacyConfigFileName, err)
	}

	fmt.Printf("Merged %s into %s (extensions.clickup)\n", config.LegacyConfigFileName, config.BeansConfigFileName)
	fmt.Printf("Deleted %s\n\n", legacyPath)
	return nil
}

func init() {
	migrateCmd.Flags().BoolVar(&migrateDryRun, "dry-run", false, "Preview migration without making changes")
	migrateCmd.Flags().BoolVar(&migrateDeleteSyncFile, "delete-sync-file", false, "Delete .sync.json after successful migration")
	rootCmd.AddCommand(migrateCmd)
}
