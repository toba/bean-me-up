// Package beans provides a wrapper for the beans CLI.
package beans

import "time"

// Bean represents a bean from the beans CLI JSON output.
type Bean struct {
	ID        string     `json:"id"`
	Slug      string     `json:"slug"`
	Path      string     `json:"path"`
	Title     string     `json:"title"`
	Status    string     `json:"status"`
	Type      string     `json:"type"`
	Priority  string     `json:"priority,omitempty"`
	CreatedAt *time.Time `json:"created_at,omitempty"`
	UpdatedAt *time.Time `json:"updated_at,omitempty"`
	Body      string     `json:"body,omitempty"`
	Parent    string     `json:"parent,omitempty"`
	Blocking  []string   `json:"blocking,omitempty"`
	Tags      []string   `json:"tags,omitempty"`
	Sync      *SyncState `json:"sync,omitempty"`
}

// SyncState holds sync metadata for external integrations.
type SyncState struct {
	ClickUp *ClickUpSyncState `json:"clickup,omitempty"`
}

// ClickUpSyncState holds ClickUp-specific sync state.
type ClickUpSyncState struct {
	TaskID   string     `json:"task_id,omitempty"`
	SyncedAt *time.Time `json:"synced_at,omitempty"`
}

// GetClickUpTaskID returns the ClickUp task ID if linked.
func (b *Bean) GetClickUpTaskID() *string {
	if b.Sync == nil || b.Sync.ClickUp == nil || b.Sync.ClickUp.TaskID == "" {
		return nil
	}
	return &b.Sync.ClickUp.TaskID
}

// GetClickUpSyncedAt returns the last sync timestamp if available.
func (b *Bean) GetClickUpSyncedAt() *time.Time {
	if b.Sync == nil || b.Sync.ClickUp == nil {
		return nil
	}
	return b.Sync.ClickUp.SyncedAt
}

// NeedsClickUpSync returns true if the bean has changed since last sync.
func (b *Bean) NeedsClickUpSync() bool {
	syncedAt := b.GetClickUpSyncedAt()
	if syncedAt == nil {
		return true // Never synced
	}
	if b.UpdatedAt == nil {
		return false // No update time, assume in sync
	}
	return b.UpdatedAt.After(*syncedAt)
}
