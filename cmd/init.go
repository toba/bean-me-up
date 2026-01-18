package cmd

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"sort"
	"strings"
	"text/template"
	"time"

	"github.com/STR-Consulting/bean-me-up/internal/clickup"
	"github.com/fatih/color"
	"github.com/spf13/cobra"
)

var initOutputPath string

var initCmd = &cobra.Command{
	Use:   "init [list-id]",
	Short: "Initialize a new .beans.clickup.yml configuration",
	Long: `Initializes a new .beans.clickup.yml configuration file by fetching data from ClickUp.

This command fetches your list's statuses, custom fields, and workspace members to
generate a config file with helpful comments and examples.

The list ID can be found in the ClickUp URL when viewing a list:
  app.clickup.com/123456/v/li/987654321
                            ^^^^^^^^^
                            This is the list ID

Requires CLICKUP_TOKEN environment variable to be set.`,
	Args: cobra.MaximumNArgs(1),
	RunE: runInit,
}

func init() {
	initCmd.Flags().StringVarP(&initOutputPath, "output", "o", ".beans.clickup.yml", "Output file path")
	rootCmd.AddCommand(initCmd)
}

// Color helpers
var (
	colorRed    = color.New(color.FgRed)
	colorYellow = color.New(color.FgYellow)
	colorCyan   = color.New(color.FgCyan)
	colorGreen  = color.New(color.FgGreen)
	colorBold   = color.New(color.Bold)
)

// configTemplateData holds the data for the config template.
type configTemplateData struct {
	ListID       string
	ListName     string
	Users        []userEntry
	Statuses     []string
	CustomFields []fieldEntry
	CustomItems  []customItemEntry
}

type customItemEntry struct {
	Name string
	ID   int
}

type userEntry struct {
	Username string
	ID       int
	Email    string
}

type fieldEntry struct {
	Name string
	Type string
	ID   string
}

func runInit(cmd *cobra.Command, args []string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Check for CLICKUP_TOKEN
	token := os.Getenv("CLICKUP_TOKEN")
	if token == "" {
		_, _ = colorRed.Fprintln(os.Stderr, "Error: CLICKUP_TOKEN environment variable is not set")
		fmt.Fprintln(os.Stderr)
		fmt.Fprintln(os.Stderr, "Get your API token from: https://app.clickup.com/settings/apps")
		fmt.Fprintln(os.Stderr, "Then run: export CLICKUP_TOKEN=\"pk_your_token\"")
		return fmt.Errorf("CLICKUP_TOKEN not set")
	}

	// Warn if beans CLI not found
	if !checkBeansInstalled() {
		_, _ = colorYellow.Fprintln(os.Stderr, "Warning: beans CLI not found in PATH")
		fmt.Fprintln(os.Stderr, "The init command will continue, but sync commands require beans.")
		fmt.Fprintln(os.Stderr)
	}

	// Get list ID from args or prompt
	var listID string
	if len(args) > 0 {
		listID = args[0]
	} else {
		var err error
		listID, err = promptListID()
		if err != nil {
			return err
		}
	}

	// Check if output file already exists
	if _, err := os.Stat(initOutputPath); err == nil {
		_, _ = colorRed.Fprintf(os.Stderr, "Error: %s already exists\n", initOutputPath)
		fmt.Fprintln(os.Stderr)
		fmt.Fprintln(os.Stderr, "Delete the existing file or use --output to specify a different path.")
		return fmt.Errorf("config file already exists")
	}

	// Create ClickUp client
	client := clickup.NewClient(token)

	// Fetch list info (required)
	_, _ = colorCyan.Print("Fetching list info... ")
	list, err := client.GetList(ctx, listID)
	if err != nil {
		_, _ = colorRed.Println("failed")
		fmt.Fprintln(os.Stderr)
		_, _ = colorRed.Fprintln(os.Stderr, "Error: Could not fetch list")
		fmt.Fprintln(os.Stderr)
		fmt.Fprintln(os.Stderr, "Verify that:")
		fmt.Fprintln(os.Stderr, "  1. The list ID is correct (check the URL: app.clickup.com/.../li/LIST_ID)")
		fmt.Fprintln(os.Stderr, "  2. Your API token has access to this list")
		return fmt.Errorf("fetching list: %w", err)
	}
	_, _ = colorGreen.Println("done")

	// Prepare template data
	data := configTemplateData{
		ListID:   listID,
		ListName: list.Name,
	}

	// Extract statuses
	for _, s := range list.Statuses {
		data.Statuses = append(data.Statuses, s.Status)
	}

	// Fetch custom fields (optional)
	_, _ = colorCyan.Print("Fetching custom fields... ")
	fields, err := client.GetAccessibleCustomFields(ctx, listID)
	if err != nil {
		_, _ = colorYellow.Println("skipped")
		_, _ = colorYellow.Fprintf(os.Stderr, "Warning: Could not fetch custom fields: %v\n", err)
	} else {
		_, _ = colorGreen.Println("done")
		for _, f := range fields {
			data.CustomFields = append(data.CustomFields, fieldEntry{
				Name: f.Name,
				Type: f.Type,
				ID:   f.ID,
			})
		}
	}

	// Fetch workspace members (optional)
	_, _ = colorCyan.Print("Fetching workspace members... ")
	members, err := client.GetWorkspaceMembers(ctx)
	if err != nil {
		_, _ = colorYellow.Println("skipped")
		_, _ = colorYellow.Fprintf(os.Stderr, "Warning: Could not fetch workspace members: %v\n", err)
	} else {
		_, _ = colorGreen.Println("done")
		for _, m := range members {
			data.Users = append(data.Users, userEntry{
				Username: sanitizeUsername(m.Username),
				ID:       m.ID,
				Email:    m.Email,
			})
		}
		// Sort users by username for consistent output
		sort.Slice(data.Users, func(i, j int) bool {
			return data.Users[i].Username < data.Users[j].Username
		})
	}

	// Fetch custom task types (optional)
	_, _ = colorCyan.Print("Fetching custom task types... ")
	customItems, err := client.GetCustomItems(ctx)
	if err != nil {
		_, _ = colorYellow.Println("skipped")
		_, _ = colorYellow.Fprintf(os.Stderr, "Warning: Could not fetch custom task types: %v\n", err)
	} else {
		_, _ = colorGreen.Println("done")
		for _, item := range customItems {
			data.CustomItems = append(data.CustomItems, customItemEntry{
				Name: item.Name,
				ID:   item.ID,
			})
		}
		// Sort by name for consistent output
		sort.Slice(data.CustomItems, func(i, j int) bool {
			return data.CustomItems[i].Name < data.CustomItems[j].Name
		})
	}

	// Generate config file
	_, _ = colorCyan.Print("Generating config file... ")
	content, err := generateConfig(data)
	if err != nil {
		_, _ = colorRed.Println("failed")
		return fmt.Errorf("generating config: %w", err)
	}

	if err := os.WriteFile(initOutputPath, []byte(content), 0644); err != nil {
		_, _ = colorRed.Println("failed")
		return fmt.Errorf("writing config: %w", err)
	}
	_, _ = colorGreen.Println("done")

	// Print success message
	fmt.Println()
	_, _ = colorGreen.Printf("Created %s\n", initOutputPath)
	fmt.Println()
	_, _ = colorBold.Println("Next steps:")
	fmt.Println("  1. Review and customize the generated config")
	fmt.Println("  2. Adjust status_mapping to match your ClickUp workflow")
	fmt.Println("  3. Preview sync: beanup sync --dry-run")
	fmt.Println()

	return nil
}

func promptListID() (string, error) {
	_, _ = colorCyan.Print("Enter ClickUp list ID: ")
	reader := bufio.NewReader(os.Stdin)
	input, err := reader.ReadString('\n')
	if err != nil {
		return "", fmt.Errorf("reading input: %w", err)
	}
	listID := strings.TrimSpace(input)
	if listID == "" {
		return "", fmt.Errorf("list ID is required")
	}
	return listID, nil
}

// sanitizeUsername converts a username to a valid YAML key.
// Removes spaces and special characters, converts to lowercase.
func sanitizeUsername(username string) string {
	// Convert to lowercase and replace spaces/dots with underscores
	result := strings.ToLower(username)
	result = strings.ReplaceAll(result, " ", "_")
	result = strings.ReplaceAll(result, ".", "_")
	// Remove any other non-alphanumeric characters except underscore
	var clean strings.Builder
	for _, r := range result {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '_' {
			clean.WriteRune(r)
		}
	}
	return clean.String()
}

const configTemplate = `# bean-me-up ClickUp configuration
# Generated by: beanup init

beans:
  clickup:
    # ClickUp list to sync tasks to
    # List: {{.ListName}}
    list_id: "{{.ListID}}"
{{if .Users}}
    # Workspace members for @mention support
    # Uncomment and keep only the users you need
    users:
{{- range .Users}}
      # {{.Username}}: {{.ID}}  # {{.Email}}
{{- end}}
{{end}}
    # Status mapping: bean status -> ClickUp status
    # Uncomment and customize to match your workflow
    # Available statuses on this list:
{{- range .Statuses}}
    #   - "{{.}}"
{{- end}}
    # status_mapping:
    #   draft: "backlog"
    #   todo: "to do"
    #   in-progress: "in progress"
    #   completed: "complete"
    #   scrapped: "closed"
{{if .CustomItems}}
    # Type mapping: bean type -> ClickUp custom task type ID
    # This maps bean types (bug, feature, milestone, etc.) to ClickUp task types
    # Run "beanup types" to see available task types
    # Available task types:
{{- range .CustomItems}}
    #   - "{{.Name}}": {{.ID}}
{{- end}}
    # type_mapping:
    #   bug: 1          # Bug
    #   milestone: 2    # Milestone
    #   feature: 0      # Task (default)
    #   task: 0         # Task (default)
{{end}}
{{if .CustomFields}}
    # Custom fields: map bean fields to ClickUp custom field UUIDs
    # Available custom fields on this list:
{{- range .CustomFields}}
    #   - "{{.Name}}" ({{.Type}}): {{.ID}}
{{- end}}
    # custom_fields:
    #   bean_id: "uuid-for-text-field"
    #   created_at: "uuid-for-date-field"
    #   updated_at: "uuid-for-date-field"
{{end}}
    # Optional: Control which beans are synced
    # sync_filter:
    #   exclude_status:
    #     - scrapped
`

func generateConfig(data configTemplateData) (string, error) {
	tmpl, err := template.New("config").Parse(configTemplate)
	if err != nil {
		return "", fmt.Errorf("parsing template: %w", err)
	}

	var buf strings.Builder
	if err := tmpl.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("executing template: %w", err)
	}

	return buf.String(), nil
}
