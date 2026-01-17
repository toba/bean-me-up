package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/pacer/bean-me-up/internal/beans"
	"github.com/pacer/bean-me-up/internal/clickup"
	"github.com/pacer/bean-me-up/internal/frontmatter"
	"github.com/spf13/cobra"
)

var linkCmd = &cobra.Command{
	Use:   "link <bean-id> <task-id>",
	Short: "Link a bean to an existing ClickUp task",
	Long: `Manually links a bean to an existing ClickUp task by setting the
sync.clickup.task_id field in the bean's frontmatter.

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

		// Read the bean file
		beanFilePath := getBeansPath() + "/" + bean.Path
		beanFile, err := frontmatter.Read(beanFilePath)
		if err != nil {
			return fmt.Errorf("reading bean file: %w", err)
		}

		// Check if already linked to this task
		existingTaskID := beanFile.GetClickUpTaskID()
		if existingTaskID != nil && *existingTaskID == taskID {
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

		// Update the bean file with task ID
		beanFile.SetClickUpTaskID(taskID)
		beanFile.SetClickUpSyncedAt(time.Now().UTC())

		if err := beanFile.Write(); err != nil {
			return fmt.Errorf("saving bean: %w", err)
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
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(result)
}
