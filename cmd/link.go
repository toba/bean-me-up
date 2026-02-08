package cmd

import (
	"context"
	"fmt"
	"time"

	"github.com/toba/bean-me-up/internal/beans"
	"github.com/toba/bean-me-up/internal/clickup"
	"github.com/spf13/cobra"
)

var linkCmd = &cobra.Command{
	Use:   "link <bean-id> <task-id>",
	Short: "Link a bean to an existing ClickUp task",
	Long: `Manually links a bean to an existing ClickUp task by storing
the task ID in the bean's extension metadata.

This is useful when you have an existing ClickUp task that you want to
associate with a bean, or when syncing fails and you need to fix the link.`,
	Args: cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		beanID := args[0]
		taskID := args[1]

		// Get the bean
		beansClient := beans.NewClient(getBeansPath())
		bean, err := beansClient.Get(beanID)
		if err != nil {
			return fmt.Errorf("bean not found: %s", beanID)
		}

		// Check if already linked to this task
		existingTaskID := bean.GetExtensionString(beans.PluginClickUp, beans.ExtKeyTaskID)
		if existingTaskID == taskID {
			if jsonOut {
				return outputLinkJSON(bean, taskID, "already_linked")
			}
			fmt.Printf("Skipped: %s already linked to %s\n", bean.ID, taskID)
			return nil
		}

		// Try to verify the task exists if we have a token
		token, tokenErr := getClickUpToken()
		if tokenErr == nil {
			client := clickup.NewClient(token)
			ctx := context.Background()
			if _, err := client.GetTask(ctx, taskID); err != nil {
				// Warn but don't fail
				fmt.Printf("Warning: Could not verify task %s: %v\n", taskID, err)
			}
		}

		// Set external data on the bean
		data := map[string]any{
			beans.ExtKeyTaskID:   taskID,
			beans.ExtKeySyncedAt: time.Now().UTC().Format(time.RFC3339),
		}
		if err := beansClient.SetExtensionData(beanID, beans.PluginClickUp, data); err != nil {
			return fmt.Errorf("saving sync state: %w", err)
		}

		if jsonOut {
			return outputLinkJSON(bean, taskID, "linked")
		}

		fmt.Printf("Linked: %s → %s\n", bean.ID, taskID)
		return nil
	},
}

func init() {
	rootCmd.AddCommand(linkCmd)
}

func outputLinkJSON(bean *beans.Bean, taskID, action string) error {
	result := map[string]string{
		"bean_id":    bean.ID,
		"bean_title": bean.Title,
		"task_id":    taskID,
		"action":     action,
	}
	return outputJSON(result)
}
