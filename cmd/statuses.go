package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/pacer/bean-me-up/internal/clickup"
	"github.com/spf13/cobra"
)

var statusesCmd = &cobra.Command{
	Use:   "statuses",
	Short: "List available statuses for the configured ClickUp list",
	Long: `Lists all statuses available on the configured ClickUp list.

Use this command to find the exact status names for configuring
status_mapping in your .bean-me-up.yml configuration.

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

		// Create client
		client := clickup.NewClient(token)

		// Fetch list info (includes statuses)
		list, err := client.GetList(ctx, cfg.ClickUp.ListID)
		if err != nil {
			return fmt.Errorf("fetching list: %w", err)
		}

		if jsonOut {
			return outputStatusesJSON(list.Statuses)
		}

		return outputStatusesText(list.Name, list.Statuses)
	},
}

func outputStatusesJSON(statuses []clickup.Status) error {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(statuses)
}

func outputStatusesText(listName string, statuses []clickup.Status) error {
	if len(statuses) == 0 {
		fmt.Println("No statuses found on this list.")
		return nil
	}

	fmt.Printf("Statuses for list %q:\n\n", listName)

	for _, s := range statuses {
		fmt.Printf("  %s (%s)\n", s.Status, s.Color)
	}

	fmt.Println()
	fmt.Println("Add status_mapping to your .bean-me-up.yml:")
	fmt.Println()
	fmt.Println("  clickup:")
	fmt.Println("    status_mapping:")
	fmt.Println("      draft: \"" + suggestStatus(statuses, "not started", "backlog", "open") + "\"")
	fmt.Println("      todo: \"" + suggestStatus(statuses, "not started", "to do", "open") + "\"")
	fmt.Println("      in-progress: \"" + suggestStatus(statuses, "in progress", "active", "doing") + "\"")
	fmt.Println("      completed: \"" + suggestStatus(statuses, "completed", "complete", "done", "closed") + "\"")

	return nil
}

// suggestStatus finds the first matching status from candidates (case-insensitive)
func suggestStatus(statuses []clickup.Status, candidates ...string) string {
	for _, candidate := range candidates {
		for _, s := range statuses {
			if strings.EqualFold(s.Status, candidate) {
				return s.Status
			}
		}
	}
	// Return first status as fallback
	if len(statuses) > 0 {
		return statuses[0].Status
	}
	return "STATUS_NAME"
}

func init() {
	rootCmd.AddCommand(statusesCmd)
}
