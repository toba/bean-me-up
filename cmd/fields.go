package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/pacer/bean-me-up/internal/clickup"
	"github.com/spf13/cobra"
)

var fieldsCmd = &cobra.Command{
	Use:   "fields",
	Short: "List available custom fields for the configured ClickUp list",
	Long: `Lists all custom fields available on the configured ClickUp list.

Use this command to find the UUIDs of custom fields that you want to map
in your .bean-me-up.yml configuration.

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

		// Fetch custom fields
		fields, err := client.GetAccessibleCustomFields(ctx, cfg.ClickUp.ListID)
		if err != nil {
			return fmt.Errorf("fetching custom fields: %w", err)
		}

		if jsonOut {
			return outputFieldsJSON(fields)
		}

		return outputFieldsText(fields)
	},
}

func outputFieldsJSON(fields []clickup.FieldInfo) error {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(fields)
}

func outputFieldsText(fields []clickup.FieldInfo) error {
	if len(fields) == 0 {
		fmt.Println("No custom fields found on this list.")
		fmt.Println("Create custom fields in ClickUp, then run this command again.")
		return nil
	}

	fmt.Printf("Custom fields for list:\n\n")

	for _, f := range fields {
		fmt.Printf("%s (%s)\n", f.Name, f.Type)
		fmt.Printf("  ID: %s\n", f.ID)
		if f.Required {
			fmt.Printf("  Required: yes\n")
		}
		fmt.Println()
	}

	fmt.Println("Add these IDs to your .bean-me-up.yml:")
	fmt.Println()
	fmt.Println("  clickup:")
	fmt.Println("    custom_fields:")
	fmt.Println("      bean_id: \"<text-field-id>\"")
	fmt.Println("      created_at: \"<date-field-id>\"")
	fmt.Println("      updated_at: \"<date-field-id>\"")

	return nil
}

func init() {
	rootCmd.AddCommand(fieldsCmd)
}
