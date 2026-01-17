package clickup

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/pacer/bean-me-up/internal/beans"
	"github.com/pacer/bean-me-up/internal/config"
	"github.com/pacer/bean-me-up/internal/frontmatter"
)

// mentionPattern matches @username patterns in text.
var mentionPattern = regexp.MustCompile(`@([a-zA-Z0-9_-]+)`)

// SyncResult holds the result of syncing a single bean.
type SyncResult struct {
	BeanID    string
	BeanTitle string
	TaskID    string
	TaskURL   string
	Action    string // "created", "updated", "skipped", "error"
	Error     error
}

// ProgressFunc is called when a bean sync completes.
// It receives the result and the current progress (completed count, total count).
type ProgressFunc func(result SyncResult, completed, total int)

// SyncOptions configures the sync operation.
type SyncOptions struct {
	DryRun          bool
	Force           bool
	NoRelationships bool
	ListID          string
	OnProgress      ProgressFunc // Optional callback for progress updates
}

// Syncer handles syncing beans to ClickUp tasks.
type Syncer struct {
	client    *Client
	config    *config.ClickUpConfig
	opts      SyncOptions
	beansPath string // Absolute path to beans directory

	// Tracking for relationship pass
	beanToTaskID map[string]string // bean ID -> ClickUp task ID
}

// NewSyncer creates a new syncer with the given client and options.
func NewSyncer(client *Client, cfg *config.ClickUpConfig, opts SyncOptions, beansPath string) *Syncer {
	return &Syncer{
		client:       client,
		config:       cfg,
		opts:         opts,
		beansPath:    beansPath,
		beanToTaskID: make(map[string]string),
	}
}

// SyncBeans syncs a list of beans to ClickUp tasks.
// Uses a multi-pass approach:
// 1. Create/update parent tasks (beans without parents, or parents not in this sync)
// 2. Create/update child tasks with parent references
// 3. Sync blocking relationships as dependencies
func (s *Syncer) SyncBeans(ctx context.Context, beanList []beans.Bean) ([]SyncResult, error) {
	// Pre-fetch authorized user to avoid per-task API calls
	if _, err := s.client.GetAuthorizedUser(ctx); err != nil {
		// Non-fatal - will just create unassigned tasks if this fails
		_ = err
	}

	// Pre-populate mapping with already-synced beans
	for _, b := range beanList {
		taskID := b.GetClickUpTaskID()
		if taskID != nil && *taskID != "" {
			s.beanToTaskID[b.ID] = *taskID
		}
	}

	// Build a set of bean IDs being synced
	syncingIDs := make(map[string]bool)
	for _, b := range beanList {
		syncingIDs[b.ID] = true
	}

	// Separate beans into layers: parents first, then children
	// A bean is a "parent" if it has no parent, or its parent isn't being synced
	var parents, children []beans.Bean
	for _, b := range beanList {
		if b.Parent == "" || !syncingIDs[b.Parent] {
			parents = append(parents, b)
		} else {
			children = append(children, b)
		}
	}

	// Create index mapping for results
	beanIndex := make(map[string]int)
	for i, b := range beanList {
		beanIndex[b.ID] = i
	}
	results := make([]SyncResult, len(beanList))
	total := len(beanList)

	var wg sync.WaitGroup
	var mu sync.Mutex // protects beanToTaskID and completed count
	var completed int

	// Helper to report progress
	reportProgress := func(result SyncResult) {
		if s.opts.OnProgress != nil {
			mu.Lock()
			completed++
			current := completed
			mu.Unlock()
			s.opts.OnProgress(result, current, total)
		}
	}

	// Pass 1: Create/update parent tasks in parallel
	for i := range parents {
		wg.Add(1)
		go func(bean beans.Bean) {
			defer wg.Done()
			result := s.syncBean(ctx, &bean)
			idx := beanIndex[bean.ID]
			results[idx] = result

			if result.Error == nil && result.Action != "skipped" && result.TaskID != "" {
				mu.Lock()
				s.beanToTaskID[bean.ID] = result.TaskID
				mu.Unlock()
			}
			reportProgress(result)
		}(parents[i])
	}
	wg.Wait()

	// Pass 2: Create/update child tasks in parallel (parents now exist)
	for i := range children {
		wg.Add(1)
		go func(bean beans.Bean) {
			defer wg.Done()
			result := s.syncBean(ctx, &bean)
			idx := beanIndex[bean.ID]
			results[idx] = result

			if result.Error == nil && result.Action != "skipped" && result.TaskID != "" {
				mu.Lock()
				s.beanToTaskID[bean.ID] = result.TaskID
				mu.Unlock()
			}
			reportProgress(result)
		}(children[i])
	}
	wg.Wait()

	// Pass 3: Sync blocking relationships in parallel (if not disabled)
	if !s.opts.NoRelationships && !s.opts.DryRun {
		for i := range beanList {
			wg.Add(1)
			go func(bean beans.Bean) {
				defer wg.Done()
				if err := s.syncRelationships(ctx, &bean); err != nil {
					// Log but don't fail - relationships are best-effort
					_ = err
				}
			}(beanList[i])
		}
		wg.Wait()
	}

	return results, nil
}

// syncBean syncs a single bean to a ClickUp task.
func (s *Syncer) syncBean(ctx context.Context, b *beans.Bean) SyncResult {
	result := SyncResult{
		BeanID:    b.ID,
		BeanTitle: b.Title,
	}

	// Read the bean file to access/update frontmatter
	beanFilePath := s.beansPath + "/" + b.Path
	beanFile, err := frontmatter.Read(beanFilePath)
	if err != nil {
		result.Action = "error"
		result.Error = fmt.Errorf("reading bean file: %w", err)
		return result
	}

	// Build the task description (markdown, with mentions stripped)
	description := s.buildTaskDescription(b)

	// Extract mentions from original body for comment creation
	mentions := s.extractMentions(b.Body)

	// Map bean status to ClickUp status
	clickUpStatus := s.getClickUpStatus(b.Status)

	// Map bean priority to ClickUp priority
	priority := s.getClickUpPriority(b.Priority)

	// Check if already linked
	taskID := beanFile.GetClickUpTaskID()
	if taskID != nil && *taskID != "" {
		result.TaskID = *taskID

		// Check if bean has changed since last sync
		if !s.opts.Force && !s.needsSync(b, beanFile) {
			result.Action = "skipped"
			return result
		}

		// Verify task still exists
		task, err := s.client.GetTask(ctx, *taskID)
		if err != nil {
			// Check if task was deleted - if so, unlink and create new
			if strings.Contains(err.Error(), "Task not found") || strings.Contains(err.Error(), "ITEM_013") {
				beanFile.ClearClickUpSync()
				if writeErr := beanFile.Write(); writeErr != nil {
					result.Action = "error"
					result.Error = fmt.Errorf("unlinking deleted task: %w", writeErr)
					return result
				}
				// Fall through to create new task below
			} else {
				result.Action = "error"
				result.Error = fmt.Errorf("fetching task %s: %w", *taskID, err)
				return result
			}
		} else {
			// Task exists - update it
			result.TaskURL = task.URL

			if s.opts.DryRun {
				result.Action = "would update"
				return result
			}

			// Update the task using markdown_description
			update := &UpdateTaskRequest{
				Name:                &b.Title,
				MarkdownDescription: &description,
				Priority:            priority,
			}
			if clickUpStatus != "" {
				update.Status = &clickUpStatus
			}

			updatedTask, err := s.client.UpdateTask(ctx, *taskID, update)
			if err != nil {
				result.Action = "error"
				result.Error = fmt.Errorf("updating task: %w", err)
				return result
			}

			// Update custom fields (best-effort)
			if err := s.updateCustomFields(ctx, *taskID, b); err != nil {
				// Log but don't fail
				_ = err
			}

			// Update synced_at timestamp
			beanFile.SetClickUpSyncedAt(time.Now().UTC())
			if err := beanFile.Write(); err != nil {
				// Log but don't fail - sync succeeded, just couldn't save timestamp
				_ = err
			}

			result.TaskURL = updatedTask.URL
			result.Action = "updated"
			return result
		}
	}

	// Create new task
	if s.opts.DryRun {
		result.Action = "would create"
		return result
	}

	createReq := &CreateTaskRequest{
		Name:                b.Title,
		MarkdownDescription: description,
		Status:              clickUpStatus,
		Priority:            priority,
		Assignees:           s.getAssignees(ctx),
		CustomFields:        s.buildCustomFields(b),
	}

	// Set parent task ID if bean has a parent that's already synced
	if b.Parent != "" {
		if parentTaskID, ok := s.beanToTaskID[b.Parent]; ok {
			createReq.Parent = &parentTaskID
		}
	}

	task, err := s.client.CreateTask(ctx, s.opts.ListID, createReq)
	if err != nil {
		result.Action = "error"
		result.Error = fmt.Errorf("creating task: %w", err)
		return result
	}

	result.TaskID = task.ID
	result.TaskURL = task.URL
	s.beanToTaskID[b.ID] = task.ID

	// Create mention comment if there are @mentions in the body
	if len(mentions) > 0 {
		if err := s.createMentionComment(ctx, task.ID, mentions); err != nil {
			// Log but don't fail - mentions are best-effort
			_ = err
		}
	}

	// Update bean file with task ID and sync timestamp
	beanFile.SetClickUpTaskID(task.ID)
	beanFile.SetClickUpSyncedAt(time.Now().UTC())
	if err := beanFile.Write(); err != nil {
		result.Action = "error"
		result.Error = fmt.Errorf("saving bean: %w", err)
		return result
	}

	result.Action = "created"
	return result
}

// needsSync checks if a bean needs to be synced based on timestamps.
func (s *Syncer) needsSync(b *beans.Bean, bf *frontmatter.BeanFile) bool {
	syncedAt := bf.GetClickUpSyncedAt()
	if syncedAt == nil {
		return true // Never synced
	}
	if b.UpdatedAt == nil {
		return false // No update time, assume in sync
	}
	return b.UpdatedAt.After(*syncedAt)
}

// buildTaskDescription builds the ClickUp task markdown description from a bean.
// Note: @mentions are stripped from the body since they don't work in descriptions.
// Mentions are handled separately via task comments.
func (s *Syncer) buildTaskDescription(b *beans.Bean) string {
	// Return the bean body with mentions stripped
	return s.stripMentions(b.Body)
}

// extractMentions finds all @username mentions in the text.
// Returns unique usernames found (without the @ prefix).
func (s *Syncer) extractMentions(text string) []string {
	matches := mentionPattern.FindAllStringSubmatch(text, -1)
	if len(matches) == 0 {
		return nil
	}

	seen := make(map[string]bool)
	var usernames []string
	for _, match := range matches {
		username := match[1]
		if !seen[username] {
			seen[username] = true
			usernames = append(usernames, username)
		}
	}
	return usernames
}

// stripMentions removes @username patterns from text.
func (s *Syncer) stripMentions(text string) string {
	return mentionPattern.ReplaceAllString(text, "")
}

// getClickUpPriority maps a bean priority to a ClickUp priority value.
// Returns nil if no mapping exists (bean has no priority or unknown priority).
func (s *Syncer) getClickUpPriority(beanPriority string) *int {
	if beanPriority == "" {
		return nil
	}

	// Use custom mapping if configured
	if s.config != nil && s.config.PriorityMapping != nil {
		if priority, ok := s.config.PriorityMapping[beanPriority]; ok {
			return &priority
		}
	}

	// Fall back to default mapping
	if priority, ok := config.DefaultPriorityMapping[beanPriority]; ok {
		return &priority
	}

	return nil
}

// buildCustomFields builds the custom fields array for task creation.
func (s *Syncer) buildCustomFields(b *beans.Bean) []CustomField {
	if s.config == nil || s.config.CustomFields == nil {
		return nil
	}

	var fields []CustomField
	cf := s.config.CustomFields

	// Bean ID field (text)
	if cf.BeanID != "" {
		fields = append(fields, CustomField{
			ID:    cf.BeanID,
			Value: b.ID,
		})
	}

	// Created at field (date - Unix milliseconds)
	if cf.CreatedAt != "" && b.CreatedAt != nil {
		fields = append(fields, CustomField{
			ID:    cf.CreatedAt,
			Value: b.CreatedAt.UnixMilli(),
		})
	}

	// Updated at field (date - Unix milliseconds)
	if cf.UpdatedAt != "" && b.UpdatedAt != nil {
		fields = append(fields, CustomField{
			ID:    cf.UpdatedAt,
			Value: b.UpdatedAt.UnixMilli(),
		})
	}

	return fields
}

// createMentionComment creates a task comment with @mentions for the given usernames.
// Only creates a comment if there are valid user mappings configured.
func (s *Syncer) createMentionComment(ctx context.Context, taskID string, usernames []string) error {
	if s.config == nil || s.config.Users == nil || len(usernames) == 0 {
		return nil
	}

	// Build comment items with mentions
	var items []CommentItem
	items = append(items, CommentItem{Text: "Mentioned: "})

	mentionCount := 0
	for i, username := range usernames {
		userID, ok := s.config.Users[username]
		if !ok {
			continue // Skip unmapped users
		}

		if i > 0 && mentionCount > 0 {
			items = append(items, CommentItem{Text: " "})
		}

		items = append(items, CommentItem{
			Type: "tag",
			User: &CommentUser{ID: userID},
		})
		mentionCount++
	}

	// Only create comment if we have at least one valid mention
	if mentionCount == 0 {
		return nil
	}

	return s.client.CreateTaskComment(ctx, taskID, items)
}

// getAssignees returns the assignee list for task creation.
// Returns token owner by default, configured assignee if set, or empty if assignee is 0.
func (s *Syncer) getAssignees(ctx context.Context) []int {
	// Check if explicitly configured
	if s.config != nil && s.config.Assignee != nil {
		if *s.config.Assignee == 0 {
			// Explicitly set to 0 means unassigned
			return nil
		}
		return []int{*s.config.Assignee}
	}

	// Default: assign to token owner
	user, err := s.client.GetAuthorizedUser(ctx)
	if err != nil {
		// Can't get user, leave unassigned
		return nil
	}
	return []int{user.ID}
}

// updateCustomFields updates custom fields on an existing task.
func (s *Syncer) updateCustomFields(ctx context.Context, taskID string, b *beans.Bean) error {
	if s.config == nil || s.config.CustomFields == nil {
		return nil
	}

	cf := s.config.CustomFields

	// Bean ID field (text)
	if cf.BeanID != "" {
		if err := s.client.SetCustomFieldValue(ctx, taskID, cf.BeanID, b.ID); err != nil {
			return fmt.Errorf("setting bean_id field: %w", err)
		}
	}

	// Created at field (date - Unix milliseconds)
	if cf.CreatedAt != "" && b.CreatedAt != nil {
		if err := s.client.SetCustomFieldValue(ctx, taskID, cf.CreatedAt, b.CreatedAt.UnixMilli()); err != nil {
			return fmt.Errorf("setting created_at field: %w", err)
		}
	}

	// Updated at field (date - Unix milliseconds)
	if cf.UpdatedAt != "" && b.UpdatedAt != nil {
		if err := s.client.SetCustomFieldValue(ctx, taskID, cf.UpdatedAt, b.UpdatedAt.UnixMilli()); err != nil {
			return fmt.Errorf("setting updated_at field: %w", err)
		}
	}

	return nil
}

// getClickUpStatus maps a bean status to a ClickUp status name.
func (s *Syncer) getClickUpStatus(beanStatus string) string {
	// Use custom mapping if configured
	if s.config != nil && s.config.StatusMapping != nil {
		if status, ok := s.config.StatusMapping[beanStatus]; ok {
			return status
		}
	}

	// Fall back to default mapping
	if status, ok := config.DefaultStatusMapping[beanStatus]; ok {
		return status
	}

	return ""
}

// syncRelationships syncs parent/blocking relationships for a bean.
func (s *Syncer) syncRelationships(ctx context.Context, b *beans.Bean) error {
	taskID, ok := s.beanToTaskID[b.ID]
	if !ok {
		return nil // Bean not synced
	}

	// Sync parent relationship (subtask)
	// Note: ClickUp requires creating a subtask with the parent field set,
	// not updating an existing task to have a parent. We skip parent sync
	// for already-created tasks to avoid complications.

	// Sync blocking relationships (dependencies)
	// In beans: bean A with blocking: [B, C] means A is blocking B and C
	// In ClickUp: we set B and C as "waiting on" A (depends_on = A)
	for _, blockedID := range b.Blocking {
		blockedTaskID, ok := s.beanToTaskID[blockedID]
		if !ok {
			continue // Blocked bean not synced
		}

		// Add dependency: blockedTaskID depends on taskID (taskID blocks blockedTaskID)
		if err := s.client.AddDependency(ctx, blockedTaskID, taskID); err != nil {
			// Dependencies might fail if already exists, continue
			_ = err
		}
	}

	return nil
}

// GetStatusMapping returns the effective status mapping for display.
func GetStatusMapping(cfg *config.ClickUpConfig) map[string]string {
	if cfg != nil && cfg.StatusMapping != nil {
		return cfg.StatusMapping
	}
	return config.DefaultStatusMapping
}

// FilterBeansForSync filters beans based on sync filter configuration.
func FilterBeansForSync(beanList []beans.Bean, filter *config.SyncFilter) []beans.Bean {
	if filter == nil {
		return beanList
	}

	excludeStatus := make(map[string]bool)
	for _, s := range filter.ExcludeStatus {
		excludeStatus[s] = true
	}

	var filtered []beans.Bean
	for _, b := range beanList {
		if !excludeStatus[b.Status] {
			filtered = append(filtered, b)
		}
	}
	return filtered
}
