package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/toba/bean-me-up/internal/beans"
	"github.com/toba/bean-me-up/internal/syncstate"
	"github.com/spf13/cobra"
)

var (
	migrateDryRun         bool
	migrateDeleteSyncFile bool
)

var migrateCmd = &cobra.Command{
	Use:   "migrate",
	Short: "Migrate sync state from .sync.json to bean extension metadata",
	Long: `Migrates ClickUp sync state from the legacy .beans/.sync.json file
to beans' extension metadata system.

After migration, sync state is stored in each bean's extension data
under the "clickup" extension, eliminating the need for a separate
sync state file.

Use --dry-run to preview the migration without making changes.
Use --delete-sync-file to remove .sync.json after a successful migration.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		bp := getBeansPath()
		if bp == "" {
			bp = ".beans"
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

		// Build batch operations
		var ops []beans.ExtensionDataOp
		for beanID, beanSync := range allBeans {
			if beanSync.ClickUp == nil || beanSync.ClickUp.TaskID == "" {
				continue
			}

			data := map[string]any{
				beans.ExtKeyTaskID: beanSync.ClickUp.TaskID,
			}
			if beanSync.ClickUp.SyncedAt != nil {
				data[beans.ExtKeySyncedAt] = beanSync.ClickUp.SyncedAt.Format(time.RFC3339)
			}

			ops = append(ops, beans.ExtensionDataOp{
				BeanID: beanID,
				Name: beans.PluginClickUp,
				Data:   data,
			})
		}

		if len(ops) == 0 {
			fmt.Println("No linked beans to migrate.")
			return nil
		}

		if migrateDryRun {
			fmt.Printf("Would migrate %d bean(s):\n", len(ops))
			for _, op := range ops {
				taskID := op.Data[beans.ExtKeyTaskID]
				fmt.Printf("  %s → clickup.task_id=%v\n", op.BeanID, taskID)
			}
			if migrateDeleteSyncFile {
				fmt.Printf("\nWould delete %s\n", syncFilePath)
			}
			return nil
		}

		// Execute batch migration
		beansClient := beans.NewClient(bp)
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

func init() {
	migrateCmd.Flags().BoolVar(&migrateDryRun, "dry-run", false, "Preview migration without making changes")
	migrateCmd.Flags().BoolVar(&migrateDeleteSyncFile, "delete-sync-file", false, "Delete .sync.json after successful migration")
	rootCmd.AddCommand(migrateCmd)
}
