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

	"github.com/toba/bean-me-up/internal/clickup"
	"github.com/fatih/color"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

var initOutputPath string

var initCmd = &cobra.Command{
	Use:   "init [list-id]",
	Short: "Initialize ClickUp configuration in .beans.yml",
	Long: `Initializes ClickUp configuration by adding an extensions.clickup section to .beans.yml.

This command fetches your list's statuses, custom fields, and custom task types to
generate a config section with helpful comments and examples.

The list ID can be found in the ClickUp URL when viewing a list:
  app.clickup.com/123456/v/li/987654321
                            ^^^^^^^^^
                            This is the list ID

Requires CLICKUP_TOKEN environment variable to be set.`,
	Args: cobra.MaximumNArgs(1),
	RunE: runInit,
}

func init() {
	initCmd.Flags().StringVarP(&initOutputPath, "output", "o", ".beans.yml", "Output file path")
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
	Statuses     []string
	CustomFields []fieldEntry
	CustomItems  []customItemEntry
}

type customItemEntry struct {
	Name string
	ID   int
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

	// Check if extensions.clickup already exists in the output file
	if _, err := os.Stat(initOutputPath); err == nil {
		data, readErr := os.ReadFile(initOutputPath)
		if readErr == nil {
			var existing map[string]any
			if yamlErr := yaml.Unmarshal(data, &existing); yamlErr == nil {
				if ext, ok := existing["extensions"]; ok {
					if extMap, ok := ext.(map[string]any); ok {
						if _, ok := extMap["clickup"]; ok {
							_, _ = colorRed.Fprintf(os.Stderr, "Error: extensions.clickup already exists in %s\n", initOutputPath)
							fmt.Fprintln(os.Stderr)
							fmt.Fprintln(os.Stderr, "Remove the existing extensions.clickup section first, or edit it manually.")
							return fmt.Errorf("extensions.clickup already exists")
						}
					}
				}
			}
		}
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

	// Generate config content
	_, _ = colorCyan.Print("Generating config... ")
	content, err := generateConfig(data)
	if err != nil {
		_, _ = colorRed.Println("failed")
		return fmt.Errorf("generating config: %w", err)
	}

	// Append to existing file or create new
	if _, err := os.Stat(initOutputPath); err == nil {
		// File exists — append the extensions section
		f, err := os.OpenFile(initOutputPath, os.O_APPEND|os.O_WRONLY, 0644)
		if err != nil {
			_, _ = colorRed.Println("failed")
			return fmt.Errorf("opening %s: %w", initOutputPath, err)
		}
		_, writeErr := f.WriteString("\n" + content)
		closeErr := f.Close()
		if writeErr != nil {
			_, _ = colorRed.Println("failed")
			return fmt.Errorf("appending to %s: %w", initOutputPath, writeErr)
		}
		if closeErr != nil {
			_, _ = colorRed.Println("failed")
			return fmt.Errorf("closing %s: %w", initOutputPath, closeErr)
		}
	} else {
		// File doesn't exist — create it
		if err := os.WriteFile(initOutputPath, []byte(content), 0644); err != nil {
			_, _ = colorRed.Println("failed")
			return fmt.Errorf("writing %s: %w", initOutputPath, err)
		}
	}
	_, _ = colorGreen.Println("done")

	// Print success message
	fmt.Println()
	_, _ = colorGreen.Printf("Added extensions.clickup to %s\n", initOutputPath)
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

const configTemplate = `# bean-me-up ClickUp configuration
# Generated by: beanup init
extensions:
  clickup:
    # ClickUp list to sync tasks to
    # List: {{.ListName}}
    list_id: "{{.ListID}}"

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
