package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/pacer/bean-me-up/internal/beans"
	"github.com/pacer/bean-me-up/internal/clickup"
	"github.com/pacer/bean-me-up/internal/syncstate"
	"github.com/spf13/cobra"
)

var (
	syncDryRun          bool
	syncForce           bool
	syncNoRelationships bool
)

var syncCmd = &cobra.Command{
	Use:   "sync [bean-id...]",
	Short: "Sync beans to ClickUp tasks",
	Long: `Syncs beans to ClickUp tasks using the ClickUp REST API.

If bean IDs are provided, only those beans are synced. Otherwise, all beans
matching the sync filter are synced.

The sync operation:
1. Creates new ClickUp tasks for beans without a linked task
2. Updates existing tasks if the bean has changed since last sync
3. Optionally syncs blocking relationships as task dependencies

Requires CLICKUP_TOKEN environment variable to be set.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := context.Background()

		// Validate config
		if cfg.ClickUp.ListID == "" {
			return fmt.Errorf("ClickUp list_id is required in .bean-me-up.yml")
		}

		// Get ClickUp token
		token, err := getClickUpToken()
		if err != nil {
			return err
		}

		// Load sync state store
		syncStore, err := syncstate.Load(getBeansPath())
		if err != nil {
			return fmt.Errorf("loading sync state: %w", err)
		}

		// Create clients
		client := clickup.NewClient(token)
		beansClient := beans.NewClient(getBeansPath())

		// Get beans to sync
		var beanList []beans.Bean
		if len(args) > 0 {
			// Sync specific beans
			beanList, err = beansClient.GetMultiple(args)
			if err != nil {
				return fmt.Errorf("getting beans: %w", err)
			}
		} else {
			// Sync all beans matching filter
			beanList, err = beansClient.List()
			if err != nil {
				return fmt.Errorf("listing beans: %w", err)
			}
			beanList = clickup.FilterBeansForSync(beanList, cfg.ClickUp.SyncFilter)
		}

		if len(beanList) == 0 {
			if jsonOut {
				fmt.Println("[]")
				return nil
			}
			fmt.Println("No beans to sync")
			return nil
		}

		// Create syncer with progress callback
		opts := clickup.SyncOptions{
			DryRun:          syncDryRun,
			Force:           syncForce,
			NoRelationships: syncNoRelationships,
			ListID:          cfg.ClickUp.ListID,
		}

		// Show progress unless JSON output is requested
		if !jsonOut {
			fmt.Printf("Syncing %d beans to ClickUp ", len(beanList))
			opts.OnProgress = func(result clickup.SyncResult, completed, total int) {
				if result.Error != nil {
					fmt.Print("x")
				} else {
					fmt.Print(".")
				}
			}
		}

		syncer := clickup.NewSyncer(client, &cfg.ClickUp, opts, getBeansPath(), syncStore)

		// Run sync
		results, err := syncer.SyncBeans(ctx, beanList)

		// Print newline after progress dots
		if !jsonOut {
			fmt.Println()
		}
		if err != nil {
			return fmt.Errorf("sync failed: %w", err)
		}

		// Save sync state
		if !syncDryRun {
			if saveErr := syncStore.Save(); saveErr != nil {
				return fmt.Errorf("saving sync state: %w", saveErr)
			}
		}

		// Output results
		if jsonOut {
			return outputResultsJSON(results)
		}
		return outputResultsText(results)
	},
}

func init() {
	syncCmd.Flags().BoolVar(&syncDryRun, "dry-run", false, "Show what would be done without making changes")
	syncCmd.Flags().BoolVar(&syncForce, "force", false, "Force update even if unchanged")
	syncCmd.Flags().BoolVar(&syncNoRelationships, "no-relationships", false, "Skip syncing blocking relationships as dependencies")
	rootCmd.AddCommand(syncCmd)
}

func outputResultsJSON(results []clickup.SyncResult) error {
	type jsonResult struct {
		BeanID    string `json:"bean_id"`
		BeanTitle string `json:"bean_title"`
		TaskID    string `json:"task_id,omitempty"`
		TaskURL   string `json:"task_url,omitempty"`
		Action    string `json:"action"`
		Error     string `json:"error,omitempty"`
	}

	jsonResults := make([]jsonResult, len(results))
	for i, r := range results {
		jsonResults[i] = jsonResult{
			BeanID:    r.BeanID,
			BeanTitle: r.BeanTitle,
			TaskID:    r.TaskID,
			TaskURL:   r.TaskURL,
			Action:    r.Action,
		}
		if r.Error != nil {
			jsonResults[i].Error = r.Error.Error()
		}
	}

	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(jsonResults)
}

func outputResultsText(results []clickup.SyncResult) error {
	var created, updated, skipped, errors int

	for _, r := range results {
		switch r.Action {
		case "created":
			created++
			fmt.Printf("  Created: %s → %s\n", r.BeanID, r.TaskURL)
		case "updated":
			updated++
			fmt.Printf("  Updated: %s → %s\n", r.BeanID, r.TaskURL)
		case "skipped":
			skipped++
		case "would create":
			fmt.Printf("  Would create: %s - %s\n", r.BeanID, r.BeanTitle)
		case "would update":
			fmt.Printf("  Would update: %s - %s\n", r.BeanID, r.BeanTitle)
		case "error":
			errors++
			fmt.Printf("  Error: %s - %v\n", r.BeanID, r.Error)
		}
	}

	fmt.Printf("\nSummary: %d created, %d updated, %d skipped, %d errors\n",
		created, updated, skipped, errors)
	return nil
}
