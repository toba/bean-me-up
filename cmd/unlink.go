package cmd

import (
	"fmt"

	"github.com/toba/bean-me-up/internal/beans"
	"github.com/spf13/cobra"
)

var unlinkCmd = &cobra.Command{
	Use:   "unlink <bean-id>",
	Short: "Remove the link between a bean and its ClickUp task",
	Long: `Removes the ClickUp sync metadata from a bean's extension data,
unlinking it from its associated ClickUp task.

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

		// Check if linked
		taskID := bean.GetExtensionString(beans.PluginClickUp, beans.ExtKeyTaskID)
		if taskID == "" {
			if jsonOut {
				return outputUnlinkJSON(bean, "", "not_linked")
			}
			fmt.Printf("Skipped: %s is not linked to a ClickUp task\n", bean.ID)
			return nil
		}

		// Remove external data
		if err := beansClient.RemoveExtensionData(beanID, beans.PluginClickUp); err != nil {
			return fmt.Errorf("removing sync state: %w", err)
		}

		if jsonOut {
			return outputUnlinkJSON(bean, taskID, "unlinked")
		}

		fmt.Printf("Unlinked: %s (was %s)\n", bean.ID, taskID)
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
	return outputJSON(result)
}
