package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/STR-Consulting/bean-me-up/internal/beans"
	"github.com/STR-Consulting/bean-me-up/internal/clickup"
	"github.com/STR-Consulting/bean-me-up/internal/syncstate"
	"github.com/spf13/cobra"
)

var statusCmd = &cobra.Command{
	Use:   "status [bean-id...]",
	Short: "Show ClickUp sync status for beans",
	Long: `Shows the sync status of beans with their linked ClickUp tasks.

If bean IDs are provided, shows status for those beans. Otherwise, shows
status for all beans that are linked to ClickUp tasks.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := context.Background()

		// Load sync state store
		syncStore, err := syncstate.Load(getBeansPath())
		if err != nil {
			return fmt.Errorf("loading sync state: %w", err)
		}

		// Get beans to check
		beansClient := beans.NewClient(getBeansPath())
		var beanList []beans.Bean

		if len(args) > 0 {
			// Check specific beans
			beanList, err = beansClient.GetMultiple(args)
			if err != nil {
				return fmt.Errorf("getting beans: %w", err)
			}
		} else {
			// Show all linked beans
			allBeans, err := beansClient.List()
			if err != nil {
				return fmt.Errorf("listing beans: %w", err)
			}
			// Filter to only linked beans
			for _, b := range allBeans {
				if syncStore.GetTaskID(b.ID) != nil {
					beanList = append(beanList, b)
				}
			}
		}

		if len(beanList) == 0 {
			if jsonOut {
				fmt.Println("[]")
				return nil
			}
			fmt.Println("No beans are linked to ClickUp tasks")
			return nil
		}

		// Try to get ClickUp client for live status check
		var client *clickup.Client
		token, _ := getClickUpToken()
		if token != "" {
			client = clickup.NewClient(token)
		}

		// Build status info
		type statusInfo struct {
			BeanID     string  `json:"bean_id"`
			BeanTitle  string  `json:"bean_title"`
			BeanStatus string  `json:"bean_status"`
			TaskID     *string `json:"task_id,omitempty"`
			TaskStatus string  `json:"task_status,omitempty"`
			TaskURL    string  `json:"task_url,omitempty"`
			Linked     bool    `json:"linked"`
			NeedsSync  bool    `json:"needs_sync"`
		}

		statuses := make([]statusInfo, len(beanList))
		for i, b := range beanList {
			taskID := syncStore.GetTaskID(b.ID)
			syncedAt := syncStore.GetSyncedAt(b.ID)

			// Calculate needsSync using sync store timestamp
			needsSync := true
			if syncedAt != nil && b.UpdatedAt != nil {
				needsSync = b.UpdatedAt.After(*syncedAt)
			} else if syncedAt != nil {
				needsSync = false
			}

			statuses[i] = statusInfo{
				BeanID:     b.ID,
				BeanTitle:  b.Title,
				BeanStatus: b.Status,
				TaskID:     taskID,
				Linked:     taskID != nil,
				NeedsSync:  needsSync,
			}

			// Fetch live task status if we have a client and task ID
			if client != nil && taskID != nil && *taskID != "" {
				// Skip archived beans (completed, scrapped)
				if b.Status == "completed" || b.Status == "scrapped" {
					continue
				}
				task, err := client.GetTask(ctx, *taskID)
				if err == nil {
					statuses[i].TaskStatus = task.Status.Status
					statuses[i].TaskURL = task.URL
				}
			}
		}

		if jsonOut {
			enc := json.NewEncoder(os.Stdout)
			enc.SetIndent("", "  ")
			return enc.Encode(statuses)
		}

		// Text output
		fmt.Printf("%-15s %-15s %-15s %-15s %s\n",
			"Bean ID", "Status", "Task ID", "Task Status", "Title")
		fmt.Println("───────────────────────────────────────────────────────────────────────────────────")

		for _, s := range statuses {
			taskStr := "-"
			taskStatusStr := "-"
			if s.TaskID != nil {
				taskStr = *s.TaskID
				if len(taskStr) > 12 {
					taskStr = taskStr[:12] + "..."
				}
			}
			if s.TaskStatus != "" {
				taskStatusStr = s.TaskStatus
			}

			title := s.BeanTitle
			if len(title) > 40 {
				title = title[:37] + "..."
			}

			fmt.Printf("%-15s %-15s %-15s %-15s %s\n",
				s.BeanID,
				s.BeanStatus,
				taskStr,
				taskStatusStr,
				title)
		}

		return nil
	},
}

func init() {
	rootCmd.AddCommand(statusCmd)
}
