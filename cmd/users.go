package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/STR-Consulting/bean-me-up/internal/clickup"
	"github.com/spf13/cobra"
)

var usersCmd = &cobra.Command{
	Use:   "users",
	Short: "List workspace members and their user IDs",
	Long: `Lists all members from accessible ClickUp workspaces.

Use this command to find user IDs for configuring @mention mappings
in your .bean-me-up.yml configuration.

Requires CLICKUP_TOKEN environment variable to be set.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := context.Background()

		// Get ClickUp token
		token, err := getClickUpToken()
		if err != nil {
			return err
		}

		// Create client
		client := clickup.NewClient(token)

		// Fetch workspace members
		members, err := client.GetWorkspaceMembers(ctx)
		if err != nil {
			return fmt.Errorf("fetching workspace members: %w", err)
		}

		if jsonOut {
			return outputUsersJSON(members)
		}

		return outputUsersText(members)
	},
}

func outputUsersJSON(members []clickup.Member) error {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(members)
}

func outputUsersText(members []clickup.Member) error {
	if len(members) == 0 {
		fmt.Println("No workspace members found.")
		return nil
	}

	fmt.Printf("Workspace members:\n\n")

	for _, m := range members {
		fmt.Printf("%s (%s)\n", m.Username, m.Email)
		fmt.Printf("  ID: %d\n", m.ID)
		fmt.Println()
	}

	fmt.Println("Add these to your .bean-me-up.yml for @mention support:")
	fmt.Println()
	fmt.Println("  clickup:")
	fmt.Println("    users:")
	for _, m := range members {
		// Extract shortname from email (part before @)
		shortname := m.Email
		if idx := strings.Index(m.Email, "@"); idx > 0 {
			shortname = m.Email[:idx]
		}
		fmt.Printf("      %s: %d  # %s\n", shortname, m.ID, m.Username)
	}

	return nil
}

func init() {
	rootCmd.AddCommand(usersCmd)
}
