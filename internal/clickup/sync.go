package clickup

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/toba/bean-me-up/internal/beans"
	"github.com/toba/bean-me-up/internal/config"
)

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
	syncStore SyncStateProvider

	// Tracking for relationship pass
	beanToTaskID map[string]string // bean ID -> ClickUp task ID

	// Space ID for space-level tag management
	spaceID string
}

// NewSyncer creates a new syncer with the given client and options.
func NewSyncer(client *Client, cfg *config.ClickUpConfig, opts SyncOptions, beansPath string, syncStore SyncStateProvider) *Syncer {
	return &Syncer{
		client:       client,
		config:       cfg,
		opts:         opts,
		beansPath:    beansPath,
		syncStore:    syncStore,
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

	// Pre-fetch list info for space ID, then populate space tag cache
	if list, err := s.client.GetList(ctx, s.opts.ListID); err == nil && list.SpaceID != "" {
		s.spaceID = list.SpaceID
		if err := s.client.PopulateSpaceTagCache(ctx, s.spaceID); err != nil {
			// Non-fatal - tags will still be added at task level
			_ = err
		}
	}

	// Pre-populate mapping with already-synced beans from sync store
	for _, b := range beanList {
		taskID := s.syncStore.GetTaskID(b.ID)
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
	for _, bean := range parents {
		wg.Go(func() {
			result := s.syncBean(ctx, &bean)
			idx := beanIndex[bean.ID]
			results[idx] = result

			if result.Error == nil && result.Action != "skipped" && result.TaskID != "" {
				mu.Lock()
				s.beanToTaskID[bean.ID] = result.TaskID
				mu.Unlock()
			}
			reportProgress(result)
		})
	}
	wg.Wait()

	// Pass 2: Create/update child tasks in parallel (parents now exist)
	for _, bean := range children {
		wg.Go(func() {
			result := s.syncBean(ctx, &bean)
			idx := beanIndex[bean.ID]
			results[idx] = result

			if result.Error == nil && result.Action != "skipped" && result.TaskID != "" {
				mu.Lock()
				s.beanToTaskID[bean.ID] = result.TaskID
				mu.Unlock()
			}
			reportProgress(result)
		})
	}
	wg.Wait()

	// Pass 3: Sync blocking relationships in parallel (if not disabled)
	if !s.opts.NoRelationships && !s.opts.DryRun {
		for _, bean := range beanList {
			wg.Go(func() {
				if err := s.syncRelationships(ctx, &bean); err != nil {
					// Log but don't fail - relationships are best-effort
					_ = err
				}
			})
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

	// Build the task description
	description := s.buildTaskDescription(b)

	// Map bean status to ClickUp status
	clickUpStatus := s.getClickUpStatus(b.Status)

	// Map bean priority to ClickUp priority
	priority := s.getClickUpPriority(b.Priority)

	// Check if already linked (from sync store)
	taskID := s.syncStore.GetTaskID(b.ID)
	if taskID != nil && *taskID != "" {
		result.TaskID = *taskID

		// Check if bean has changed since last sync
		if !s.opts.Force && !s.needsSync(b) {
			result.Action = "skipped"
			return result
		}

		// Verify task still exists
		task, err := s.client.GetTask(ctx, *taskID)
		if err != nil {
			// Check if task was deleted - if so, unlink and create new
			if strings.Contains(err.Error(), "Task not found") || strings.Contains(err.Error(), "ITEM_013") {
				s.syncStore.Clear(b.ID)
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

			// Build update request with only changed fields
			update := s.buildUpdateRequest(task, b, description, priority, clickUpStatus)

			// Check if any core fields changed
			if update.hasChanges() {
				updatedTask, err := s.client.UpdateTask(ctx, *taskID, update)
				if err != nil {
					result.Action = "error"
					result.Error = fmt.Errorf("updating task: %w", err)
					return result
				}
				result.TaskURL = updatedTask.URL
			}

			// Update custom fields only if changed (best-effort)
			customFieldsUpdated := s.updateChangedCustomFields(ctx, task, *taskID, b)

			// Sync tags (best-effort)
			tagsChanged := s.syncTags(ctx, *taskID, b, task.Tags)

			// Update synced_at timestamp in sync store
			s.syncStore.SetSyncedAt(b.ID, time.Now().UTC())

			if update.hasChanges() || customFieldsUpdated || tagsChanged {
				result.Action = "updated"
			} else {
				result.Action = "unchanged"
			}
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
		CustomItemID:        s.getClickUpCustomItemID(b.Type),
	}

	// Set due date if bean has one
	if b.Due != nil {
		if dueTime, err := parseBeanDueDate(*b.Due); err == nil {
			millis := toLocalDateMillis(dueTime)
			createReq.DueDate = &millis
			createReq.DueDatetime = ptrBool(false)
		}
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

	// Sync tags for new task (no existing tags to remove)
	s.syncTags(ctx, task.ID, b, nil)

	// Store task ID and sync timestamp in sync store
	s.syncStore.SetTaskID(b.ID, task.ID)
	s.syncStore.SetSyncedAt(b.ID, time.Now().UTC())

	result.Action = "created"
	return result
}

// needsSync checks if a bean needs to be synced based on timestamps.
func (s *Syncer) needsSync(b *beans.Bean) bool {
	syncedAt := s.syncStore.GetSyncedAt(b.ID)
	if syncedAt == nil {
		return true // Never synced
	}
	if b.UpdatedAt == nil {
		return false // No update time, assume in sync
	}
	return b.UpdatedAt.After(*syncedAt)
}

// buildTaskDescription builds the ClickUp task markdown description from a bean.
func (s *Syncer) buildTaskDescription(b *beans.Bean) string {
	return b.Body
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
	// Convert to local date at midnight to avoid timezone display issues in ClickUp
	if cf.CreatedAt != "" && b.CreatedAt != nil {
		fields = append(fields, CustomField{
			ID:    cf.CreatedAt,
			Value: toLocalDateMillis(*b.CreatedAt),
		})
	}

	// Updated at field (date - Unix milliseconds)
	// Convert to local date at midnight to avoid timezone display issues in ClickUp
	if cf.UpdatedAt != "" && b.UpdatedAt != nil {
		fields = append(fields, CustomField{
			ID:    cf.UpdatedAt,
			Value: toLocalDateMillis(*b.UpdatedAt),
		})
	}

	return fields
}

// toLocalDateMillis converts a timestamp to midnight of that date in local timezone.
// This ensures ClickUp displays the date the user expects (the local date when the bean
// was created) rather than potentially showing "Tomorrow" due to UTC offset.
func toLocalDateMillis(t time.Time) int64 {
	local := t.Local()
	midnight := time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, time.Local)
	return midnight.UnixMilli()
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

// buildUpdateRequest builds an UpdateTaskRequest containing only fields that differ from current.
func (s *Syncer) buildUpdateRequest(current *TaskInfo, b *beans.Bean, description string, priority *int, clickUpStatus string) *UpdateTaskRequest {
	update := &UpdateTaskRequest{}

	// Only include name if changed
	if current.Name != b.Title {
		update.Name = &b.Title
	}

	// Only include description if changed
	if current.Description != description {
		update.MarkdownDescription = &description
	}

	// Only include priority if changed
	if !s.priorityEqual(current.Priority, priority) {
		update.Priority = priority
	}

	// Only include status if changed
	if clickUpStatus != "" && current.Status.Status != clickUpStatus {
		update.Status = &clickUpStatus
	}

	// Only include due date if changed
	newDueMillis := beanDueToMillis(b.Due)
	currentDueMillis := clickUpDueToMillis(current.DueDate)
	if !int64PtrEqual(currentDueMillis, newDueMillis) {
		if newDueMillis != nil {
			update.DueDate = newDueMillis
			update.DueDatetime = ptrBool(false)
		} else {
			// Clear due date: ClickUp accepts null to remove it
			zero := int64(0)
			update.DueDate = &zero
		}
	}

	// Only include custom item ID if changed
	newItemID := s.getClickUpCustomItemID(b.Type)
	if !intPtrEqual(current.CustomItemID, newItemID) {
		update.CustomItemID = newItemID
	}

	return update
}

// priorityEqual compares a TaskPriority (from ClickUp response) with a target priority int pointer.
func (s *Syncer) priorityEqual(current *TaskPriority, target *int) bool {
	if current == nil && target == nil {
		return true
	}
	if current == nil || target == nil {
		return false
	}
	return current.ID == *target
}

// intPtrEqual compares two int pointers for equality.
func intPtrEqual(a, b *int) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	return *a == *b
}

// updateChangedCustomFields updates only custom fields that have changed.
// Returns true if any field was updated.
func (s *Syncer) updateChangedCustomFields(ctx context.Context, current *TaskInfo, taskID string, b *beans.Bean) bool {
	if s.config == nil || s.config.CustomFields == nil {
		return false
	}

	cf := s.config.CustomFields
	updated := false

	// Build a map of current custom field values by ID for quick lookup
	currentFields := make(map[string]any)
	for _, f := range current.CustomFields {
		currentFields[f.ID] = f.Value
	}

	// Bean ID field (text)
	if cf.BeanID != "" {
		currentVal, _ := currentFields[cf.BeanID].(string)
		if currentVal != b.ID {
			if err := s.client.SetCustomFieldValue(ctx, taskID, cf.BeanID, b.ID); err == nil {
				updated = true
			}
		}
	}

	// Created at field (date - Unix milliseconds)
	if cf.CreatedAt != "" && b.CreatedAt != nil {
		newVal := toLocalDateMillis(*b.CreatedAt)
		if !customFieldDateEqual(currentFields[cf.CreatedAt], newVal) {
			if err := s.client.SetCustomFieldValue(ctx, taskID, cf.CreatedAt, newVal); err == nil {
				updated = true
			}
		}
	}

	// Updated at field (date - Unix milliseconds)
	if cf.UpdatedAt != "" && b.UpdatedAt != nil {
		newVal := toLocalDateMillis(*b.UpdatedAt)
		if !customFieldDateEqual(currentFields[cf.UpdatedAt], newVal) {
			if err := s.client.SetCustomFieldValue(ctx, taskID, cf.UpdatedAt, newVal); err == nil {
				updated = true
			}
		}
	}

	return updated
}

// customFieldDateEqual compares a custom field date value (from ClickUp, can be string or number)
// with a target milliseconds value.
func customFieldDateEqual(current any, target int64) bool {
	if current == nil {
		return false
	}

	// ClickUp returns date fields as string timestamps in milliseconds
	switch v := current.(type) {
	case string:
		// Parse string to int64
		var parsed int64
		if _, err := fmt.Sscanf(v, "%d", &parsed); err == nil {
			return parsed == target
		}
	case float64:
		// JSON numbers are decoded as float64
		return int64(v) == target
	case int64:
		return v == target
	}
	return false
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

// getClickUpCustomItemID maps a bean type to a ClickUp custom item ID.
// Returns nil if no mapping exists (task will use default type).
func (s *Syncer) getClickUpCustomItemID(beanType string) *int {
	if beanType == "" {
		return nil
	}

	// Use custom mapping if configured
	if s.config != nil && s.config.TypeMapping != nil {
		if customItemID, ok := s.config.TypeMapping[beanType]; ok {
			return &customItemID
		}
	}

	return nil
}

// syncTags syncs bean tags to ClickUp task tags.
// Returns true if any tags were added or removed.
func (s *Syncer) syncTags(ctx context.Context, taskID string, b *beans.Bean, currentTags []Tag) bool {
	// Build set of current ClickUp tag names
	current := make(map[string]bool)
	for _, t := range currentTags {
		current[t.Name] = true
	}

	// Build set of desired bean tag names
	desired := make(map[string]bool)
	for _, t := range b.Tags {
		desired[t] = true
	}

	changed := false

	// Add missing tags
	for _, t := range b.Tags {
		if !current[t] {
			// Ensure tag exists at space level so it's discoverable in the tag picker
			if s.spaceID != "" {
				if err := s.client.EnsureSpaceTag(ctx, s.spaceID, t); err != nil {
					_ = err // Best-effort
				}
			}
			if err := s.client.AddTagToTask(ctx, taskID, t); err != nil {
				_ = err // Best-effort
			} else {
				changed = true
			}
		}
	}

	// Remove extra tags
	for _, t := range currentTags {
		if !desired[t.Name] {
			if err := s.client.RemoveTagFromTask(ctx, taskID, t.Name); err != nil {
				_ = err // Best-effort
			} else {
				changed = true
			}
		}
	}

	return changed
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

// FilterBeansNeedingSync returns only beans that need to be synced based on timestamps.
// A bean needs sync if: force is true, it has no sync record, or it was updated after last sync.
func FilterBeansNeedingSync(beanList []beans.Bean, store SyncStateProvider, force bool) []beans.Bean {
	var needSync []beans.Bean
	for _, b := range beanList {
		if force {
			needSync = append(needSync, b)
			continue
		}
		syncedAt := store.GetSyncedAt(b.ID)
		if syncedAt == nil {
			needSync = append(needSync, b) // Never synced
			continue
		}
		if b.UpdatedAt != nil && b.UpdatedAt.After(*syncedAt) {
			needSync = append(needSync, b) // Updated since last sync
		}
	}
	return needSync
}

// parseBeanDueDate parses a bean due date string ("YYYY-MM-DD") into a time.Time.
func parseBeanDueDate(s string) (time.Time, error) {
	return time.ParseInLocation("2006-01-02", s, time.Local)
}

// beanDueToMillis converts a bean due date string to Unix milliseconds (local midnight).
// Returns nil if the bean has no due date or the date is unparseable.
func beanDueToMillis(due *string) *int64 {
	if due == nil || *due == "" {
		return nil
	}
	t, err := parseBeanDueDate(*due)
	if err != nil {
		return nil
	}
	millis := toLocalDateMillis(t)
	return &millis
}

// clickUpDueToMillis parses ClickUp's due_date string (Unix ms) into an *int64.
// Returns nil if the string is nil or empty.
func clickUpDueToMillis(s *string) *int64 {
	if s == nil || *s == "" {
		return nil
	}
	var millis int64
	if _, err := fmt.Sscanf(*s, "%d", &millis); err != nil {
		return nil
	}
	return &millis
}

// int64PtrEqual compares two int64 pointers for equality.
func int64PtrEqual(a, b *int64) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	return *a == *b
}

// ptrBool returns a pointer to a bool value.
func ptrBool(v bool) *bool {
	return &v
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
