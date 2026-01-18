package cmd

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/STR-Consulting/bean-me-up/internal/clickup"
	"github.com/STR-Consulting/bean-me-up/internal/config"
	"github.com/STR-Consulting/bean-me-up/internal/syncstate"
	"github.com/spf13/cobra"
)

var skipAPI bool

var checkCmd = &cobra.Command{
	Use:   "check",
	Short: "Verify configuration and sync state health",
	Long: `Validates configuration, ClickUp connectivity, and sync state.

Checks include:
  - Configuration file exists and is parseable
  - List ID is configured and accessible
  - Status, priority, and type mappings are valid
  - Custom fields exist on the ClickUp list (if configured)
  - CLICKUP_TOKEN is set and valid
  - Sync state file is valid
  - All linked tasks exist in ClickUp

Use --skip-api to perform offline validation only.`,
	RunE: runCheck,
}

func init() {
	checkCmd.Flags().BoolVar(&skipAPI, "skip-api", false, "Skip ClickUp API checks (offline validation only)")
	rootCmd.AddCommand(checkCmd)
}

// checkStatus represents the result of a single check.
type checkStatus string

const (
	checkPass checkStatus = "pass"
	checkWarn checkStatus = "warn"
	checkFail checkStatus = "fail"
)

// checkResult holds the result of a single check.
type checkResult struct {
	Name    string      `json:"name"`
	Status  checkStatus `json:"status"`
	Message string      `json:"message"`
}

// checkSection groups related checks.
type checkSection struct {
	Name   string        `json:"name"`
	Checks []checkResult `json:"checks"`
}

// checkSummary summarizes the overall results.
type checkSummary struct {
	Passed   int `json:"passed"`
	Warnings int `json:"warnings"`
	Failed   int `json:"failed"`
}

// checkOutput is the JSON output structure.
type checkOutput struct {
	Sections []checkSection `json:"sections"`
	Summary  checkSummary   `json:"summary"`
}

func runCheck(cmd *cobra.Command, args []string) error {
	// Suppress usage on error since check errors are specific validation failures
	cmd.SilenceUsage = true

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	output := checkOutput{
		Sections: make([]checkSection, 0, 3),
	}

	// Configuration section
	configSection := checkConfiguration(ctx)
	output.Sections = append(output.Sections, configSection)

	// ClickUp Integration section
	integrationSection := checkClickUpIntegration(ctx)
	output.Sections = append(output.Sections, integrationSection)

	// Sync State section
	syncSection := checkSyncState(ctx)
	output.Sections = append(output.Sections, syncSection)

	// Calculate summary
	for _, section := range output.Sections {
		for _, check := range section.Checks {
			switch check.Status {
			case checkPass:
				output.Summary.Passed++
			case checkWarn:
				output.Summary.Warnings++
			case checkFail:
				output.Summary.Failed++
			}
		}
	}

	if jsonOut {
		return outputJSON(output)
	}

	// Text output
	printCheckOutput(output)

	// Exit with error code if any checks failed
	if output.Summary.Failed > 0 {
		return fmt.Errorf("%d check(s) failed", output.Summary.Failed)
	}

	return nil
}

func checkConfiguration(ctx context.Context) checkSection {
	section := checkSection{
		Name:   "Configuration",
		Checks: make([]checkResult, 0),
	}

	// Check if config file exists and is parseable
	cwd, err := os.Getwd()
	if err != nil {
		section.Checks = append(section.Checks, checkResult{
			Name:    "Config file found",
			Status:  checkFail,
			Message: fmt.Sprintf("Cannot get working directory: %v", err),
		})
		return section
	}

	configPath, err := config.FindConfig(cwd)
	if err != nil {
		section.Checks = append(section.Checks, checkResult{
			Name:    "Config file found",
			Status:  checkFail,
			Message: fmt.Sprintf("Error searching: %v", err),
		})
		return section
	}

	if configPath == "" {
		section.Checks = append(section.Checks, checkResult{
			Name:    "Config file found",
			Status:  checkFail,
			Message: "No .beans.clickup.yml found",
		})
		return section
	}

	cfg, err := config.Load(configPath)
	if err != nil {
		section.Checks = append(section.Checks, checkResult{
			Name:    "Config file found",
			Status:  checkFail,
			Message: fmt.Sprintf("Cannot parse: %v", err),
		})
		return section
	}

	section.Checks = append(section.Checks, checkResult{
		Name:    "Config file found",
		Status:  checkPass,
		Message: config.ConfigFileName,
	})

	// Check list_id
	listID := cfg.Beans.ClickUp.ListID
	if listID == "" {
		section.Checks = append(section.Checks, checkResult{
			Name:    "List ID configured",
			Status:  checkFail,
			Message: "list_id is not set",
		})
	} else {
		section.Checks = append(section.Checks, checkResult{
			Name:    "List ID configured",
			Status:  checkPass,
			Message: listID,
		})
	}

	// Check list accessibility (requires API)
	if !skipAPI && listID != "" {
		token, _ := getClickUpToken()
		if token != "" {
			client := clickup.NewClient(token)
			list, err := client.GetList(ctx, listID)
			if err != nil {
				section.Checks = append(section.Checks, checkResult{
					Name:    "List accessible",
					Status:  checkFail,
					Message: fmt.Sprintf("Cannot access list: %v", err),
				})
			} else {
				section.Checks = append(section.Checks, checkResult{
					Name:    "List accessible",
					Status:  checkPass,
					Message: list.Name,
				})

				// Check status mapping against list statuses
				section.Checks = append(section.Checks, checkStatusMapping(cfg, list)...)

				// Check custom fields if configured
				if cfg.Beans.ClickUp.CustomFields != nil {
					section.Checks = append(section.Checks, checkCustomFields(ctx, cfg, client, listID)...)
				} else {
					section.Checks = append(section.Checks, checkResult{
						Name:    "Custom fields configured",
						Status:  checkWarn,
						Message: "Not configured",
					})
				}
			}
		}
	}

	// Check priority mapping
	priorityMapping := cfg.GetPriorityMapping()
	invalidPriorities := []string{}
	for beanPriority, clickupPriority := range priorityMapping {
		if clickupPriority < 1 || clickupPriority > 4 {
			invalidPriorities = append(invalidPriorities, fmt.Sprintf("%s=%d", beanPriority, clickupPriority))
		}
	}
	if len(invalidPriorities) > 0 {
		section.Checks = append(section.Checks, checkResult{
			Name:    "Priority mapping valid",
			Status:  checkWarn,
			Message: fmt.Sprintf("Invalid priorities (must be 1-4): %v", invalidPriorities),
		})
	} else {
		section.Checks = append(section.Checks, checkResult{
			Name:    "Priority mapping valid",
			Status:  checkPass,
			Message: fmt.Sprintf("%d mappings", len(priorityMapping)),
		})
	}

	// Check type mapping
	if len(cfg.Beans.ClickUp.TypeMapping) > 0 {
		section.Checks = append(section.Checks, checkResult{
			Name:    "Type mapping configured",
			Status:  checkPass,
			Message: fmt.Sprintf("%d mappings", len(cfg.Beans.ClickUp.TypeMapping)),
		})
	} else {
		section.Checks = append(section.Checks, checkResult{
			Name:    "Type mapping configured",
			Status:  checkWarn,
			Message: "Not configured (bean types won't map to ClickUp task types)",
		})
	}

	// Check users
	if len(cfg.Beans.ClickUp.Users) > 0 {
		section.Checks = append(section.Checks, checkResult{
			Name:    "Users configured",
			Status:  checkPass,
			Message: fmt.Sprintf("%d users", len(cfg.Beans.ClickUp.Users)),
		})
	}

	return section
}

func checkStatusMapping(cfg *config.Config, list *clickup.List) []checkResult {
	results := make([]checkResult, 0)

	statusMapping := cfg.GetStatusMapping()
	if len(statusMapping) == 0 {
		results = append(results, checkResult{
			Name:    "Status mapping valid",
			Status:  checkWarn,
			Message: "No status mapping configured (using defaults)",
		})
		return results
	}

	// Build set of valid ClickUp statuses
	validStatuses := make(map[string]bool)
	for _, s := range list.Statuses {
		validStatuses[s.Status] = true
	}

	// Check each mapping
	invalidMappings := []string{}
	for beanStatus, clickupStatus := range statusMapping {
		if !validStatuses[clickupStatus] {
			invalidMappings = append(invalidMappings, fmt.Sprintf("%s→%s", beanStatus, clickupStatus))
		}
	}

	if len(invalidMappings) > 0 {
		results = append(results, checkResult{
			Name:    "Status mapping valid",
			Status:  checkWarn,
			Message: fmt.Sprintf("Unknown statuses: %v", invalidMappings),
		})
	} else {
		results = append(results, checkResult{
			Name:    "Status mapping valid",
			Status:  checkPass,
			Message: fmt.Sprintf("%d mappings", len(statusMapping)),
		})
	}

	return results
}

func checkCustomFields(ctx context.Context, cfg *config.Config, client *clickup.Client, listID string) []checkResult {
	results := make([]checkResult, 0)

	fields, err := client.GetAccessibleCustomFields(ctx, listID)
	if err != nil {
		results = append(results, checkResult{
			Name:    "Custom fields valid",
			Status:  checkWarn,
			Message: fmt.Sprintf("Cannot fetch fields: %v", err),
		})
		return results
	}

	// Build set of valid field IDs
	validFields := make(map[string]string) // ID -> name
	for _, f := range fields {
		validFields[f.ID] = f.Name
	}

	// Check configured fields
	cf := cfg.Beans.ClickUp.CustomFields
	invalidFields := []string{}

	if cf.BeanID != "" {
		if _, ok := validFields[cf.BeanID]; !ok {
			invalidFields = append(invalidFields, "bean_id")
		}
	}
	if cf.CreatedAt != "" {
		if _, ok := validFields[cf.CreatedAt]; !ok {
			invalidFields = append(invalidFields, "created_at")
		}
	}
	if cf.UpdatedAt != "" {
		if _, ok := validFields[cf.UpdatedAt]; !ok {
			invalidFields = append(invalidFields, "updated_at")
		}
	}

	if len(invalidFields) > 0 {
		results = append(results, checkResult{
			Name:    "Custom fields valid",
			Status:  checkWarn,
			Message: fmt.Sprintf("Unknown field UUIDs: %v", invalidFields),
		})
	} else {
		configuredCount := 0
		if cf.BeanID != "" {
			configuredCount++
		}
		if cf.CreatedAt != "" {
			configuredCount++
		}
		if cf.UpdatedAt != "" {
			configuredCount++
		}
		results = append(results, checkResult{
			Name:    "Custom fields valid",
			Status:  checkPass,
			Message: fmt.Sprintf("%d fields configured", configuredCount),
		})
	}

	return results
}

func checkClickUpIntegration(ctx context.Context) checkSection {
	section := checkSection{
		Name:   "ClickUp Integration",
		Checks: make([]checkResult, 0),
	}

	// Check CLICKUP_TOKEN
	token, err := getClickUpToken()
	if err != nil {
		section.Checks = append(section.Checks, checkResult{
			Name:    "CLICKUP_TOKEN set",
			Status:  checkFail,
			Message: "Environment variable not set",
		})
		return section
	}

	section.Checks = append(section.Checks, checkResult{
		Name:    "CLICKUP_TOKEN set",
		Status:  checkPass,
		Message: "Set",
	})

	if skipAPI {
		section.Checks = append(section.Checks, checkResult{
			Name:    "API token valid",
			Status:  checkWarn,
			Message: "Skipped (--skip-api)",
		})
		return section
	}

	// Validate token by fetching authorized user
	client := clickup.NewClient(token)
	user, err := client.GetAuthorizedUser(ctx)
	if err != nil {
		section.Checks = append(section.Checks, checkResult{
			Name:    "API token valid",
			Status:  checkFail,
			Message: fmt.Sprintf("Invalid token: %v", err),
		})
		return section
	}

	section.Checks = append(section.Checks, checkResult{
		Name:    "API token valid",
		Status:  checkPass,
		Message: user.Email,
	})

	return section
}

func checkSyncState(ctx context.Context) checkSection {
	section := checkSection{
		Name:   "Sync State",
		Checks: make([]checkResult, 0),
	}

	beansPath := getBeansPath()

	// Load sync state
	store, err := syncstate.Load(beansPath)
	if err != nil {
		section.Checks = append(section.Checks, checkResult{
			Name:    "Sync state file valid",
			Status:  checkFail,
			Message: fmt.Sprintf("Cannot load: %v", err),
		})
		return section
	}

	section.Checks = append(section.Checks, checkResult{
		Name:    "Sync state file valid",
		Status:  checkPass,
		Message: syncstate.SyncFileName,
	})

	// Count linked beans
	allBeans := store.GetAllBeans()
	linkedCount := 0
	for _, bean := range allBeans {
		if bean.ClickUp != nil && bean.ClickUp.TaskID != "" {
			linkedCount++
		}
	}

	section.Checks = append(section.Checks, checkResult{
		Name:    "Beans linked",
		Status:  checkPass,
		Message: fmt.Sprintf("%d beans", linkedCount),
	})

	if linkedCount == 0 {
		return section
	}

	// Check for stale syncs (>7 days)
	staleThreshold := time.Now().AddDate(0, 0, -7)
	staleCount := 0
	for _, bean := range allBeans {
		if bean.ClickUp != nil && bean.ClickUp.SyncedAt != nil {
			if bean.ClickUp.SyncedAt.Before(staleThreshold) {
				staleCount++
			}
		}
	}

	if staleCount > 0 {
		section.Checks = append(section.Checks, checkResult{
			Name:    "Stale syncs",
			Status:  checkWarn,
			Message: fmt.Sprintf("%d beans have stale sync (>7 days)", staleCount),
		})
	}

	// Verify linked tasks exist (if API is available)
	if !skipAPI {
		token, _ := getClickUpToken()
		if token != "" {
			client := clickup.NewClient(token)
			missingCount := 0

			for beanID, bean := range allBeans {
				if bean.ClickUp != nil && bean.ClickUp.TaskID != "" {
					_, err := client.GetTask(ctx, bean.ClickUp.TaskID)
					if err != nil {
						missingCount++
						// Only report first few missing for brevity
						if missingCount <= 3 {
							section.Checks = append(section.Checks, checkResult{
								Name:    "Task exists",
								Status:  checkWarn,
								Message: fmt.Sprintf("%s → %s: not found", beanID, bean.ClickUp.TaskID),
							})
						}
					}
				}
			}

			if missingCount == 0 {
				section.Checks = append(section.Checks, checkResult{
					Name:    "All linked tasks exist",
					Status:  checkPass,
					Message: fmt.Sprintf("Verified %d tasks", linkedCount),
				})
			} else if missingCount > 3 {
				section.Checks = append(section.Checks, checkResult{
					Name:    "Missing tasks",
					Status:  checkWarn,
					Message: fmt.Sprintf("...and %d more", missingCount-3),
				})
			}
		}
	}

	return section
}

func printCheckOutput(output checkOutput) {
	for _, section := range output.Sections {
		_, _ = colorBold.Println(section.Name)
		for _, check := range section.Checks {
			switch check.Status {
			case checkPass:
				_, _ = colorGreen.Print("  ✓ ")
			case checkWarn:
				_, _ = colorYellow.Print("  ⚠ ")
			case checkFail:
				_, _ = colorRed.Print("  ✗ ")
			}

			fmt.Print(check.Name)
			if check.Message != "" {
				_, _ = colorCyan.Printf(" (%s)", check.Message)
			}
			fmt.Println()
		}
		fmt.Println()
	}

	// Print summary
	_, _ = colorBold.Print("Summary: ")
	_, _ = colorGreen.Printf("%d passed", output.Summary.Passed)
	if output.Summary.Warnings > 0 {
		fmt.Print(", ")
		_, _ = colorYellow.Printf("%d warnings", output.Summary.Warnings)
	}
	if output.Summary.Failed > 0 {
		fmt.Print(", ")
		_, _ = colorRed.Printf("%d failed", output.Summary.Failed)
	}
	fmt.Println()
}
