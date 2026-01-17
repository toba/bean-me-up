package cmd

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/pacer/bean-me-up/internal/beans"
	"github.com/pacer/bean-me-up/internal/syncstate"
	"github.com/spf13/cobra"
)

var unlinkCmd = &cobra.Command{
	Use:   "unlink <bean-id>",
	Short: "Remove the link between a bean and its ClickUp task",
	Long: `Removes the ClickUp sync state for a bean from the sync state file
(.beans/.sync.json), unlinking it from its associated ClickUp task.

Note: This does not delete or modify the ClickUp task itself.`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		beanID := args[0]

		// Get the bean
		beansClient := beans.NewClient(getBeansPath())
		bean, err := beansClient.Get(beanID)
		if err != nil {
			return fmt.Errorf("bean not found: %s", beanID)
		}

		// Load sync state store
		syncStore, err := syncstate.Load(getBeansPath())
		if err != nil {
			return fmt.Errorf("loading sync state: %w", err)
		}

		// Check if linked
		taskID := syncStore.GetTaskID(beanID)
		if taskID == nil {
			if jsonOut {
				return outputUnlinkJSON(bean, "", "not_linked")
			}
			fmt.Printf("Skipped: %s is not linked to a ClickUp task\n", bean.ID)
			return nil
		}

		oldTaskID := *taskID

		// Clear the sync state
		syncStore.Clear(beanID)
		if err := syncStore.Save(); err != nil {
			return fmt.Errorf("saving sync state: %w", err)
		}

		if jsonOut {
			return outputUnlinkJSON(bean, oldTaskID, "unlinked")
		}

		fmt.Printf("Unlinked: %s (was %s)\n", bean.ID, oldTaskID)
		return nil
	},
}

func init() {
	rootCmd.AddCommand(unlinkCmd)
}

func outputUnlinkJSON(bean *beans.Bean, taskID, action string) error {
	result := map[string]string{
		"bean_id":    bean.ID,
		"bean_title": bean.Title,
		"action":     action,
	}
	if taskID != "" {
		result["task_id"] = taskID
	}
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(result)
}
