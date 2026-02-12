// Package syncstate manages sync state storage in a separate JSON file.
// This avoids losing sync state when the beans CLI overwrites frontmatter.
package syncstate

import (
	"encoding/json"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"sync"
	"time"
)

const (
	// SyncFileName is the name of the sync state file inside the beans directory.
	SyncFileName = ".sync.json"
	// CurrentVersion is the current schema version.
	CurrentVersion = 1
)

// SyncData is the root structure of the sync state file.
type SyncData struct {
	Version int                  `json:"version"`
	Beans   map[string]*BeanSync `json:"beans"`
}

// BeanSync holds sync state for a single bean.
type BeanSync struct {
	ClickUp *ClickUpSync `json:"clickup,omitempty"`
}

// ClickUpSync holds ClickUp-specific sync state.
type ClickUpSync struct {
	TaskID   string     `json:"task_id"`
	SyncedAt *time.Time `json:"synced_at,omitempty"`
}

// Store manages sync state persistence.
type Store struct {
	filePath string
	data     *SyncData
	mu       sync.RWMutex
}

// Load loads or creates a sync state store for the given beans path.
func Load(beansPath string) (*Store, error) {
	filePath := filepath.Join(beansPath, SyncFileName)

	store := &Store{
		filePath: filePath,
		data: &SyncData{
			Version: CurrentVersion,
			Beans:   make(map[string]*BeanSync),
		},
	}

	// Check if file exists
	data, err := os.ReadFile(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			// File doesn't exist yet, use empty state
			return store, nil
		}
		return nil, fmt.Errorf("reading sync state file: %w", err)
	}

	// Parse existing file
	if err := json.Unmarshal(data, store.data); err != nil {
		return nil, fmt.Errorf("parsing sync state file: %w", err)
	}

	// Ensure beans map is initialized
	if store.data.Beans == nil {
		store.data.Beans = make(map[string]*BeanSync)
	}

	return store, nil
}

// GetTaskID returns the ClickUp task ID for a bean, or nil if not linked.
func (s *Store) GetTaskID(beanID string) *string {
	s.mu.RLock()
	defer s.mu.RUnlock()

	bean, ok := s.data.Beans[beanID]
	if !ok || bean == nil || bean.ClickUp == nil || bean.ClickUp.TaskID == "" {
		return nil
	}
	return &bean.ClickUp.TaskID
}

// GetSyncedAt returns the last sync timestamp for a bean, or nil if never synced.
func (s *Store) GetSyncedAt(beanID string) *time.Time {
	s.mu.RLock()
	defer s.mu.RUnlock()

	bean, ok := s.data.Beans[beanID]
	if !ok || bean == nil || bean.ClickUp == nil {
		return nil
	}
	return bean.ClickUp.SyncedAt
}

// SetTaskID sets the ClickUp task ID for a bean.
func (s *Store) SetTaskID(beanID, taskID string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.ensureBean(beanID)
	s.data.Beans[beanID].ClickUp.TaskID = taskID
}

// SetSyncedAt sets the last sync timestamp for a bean.
func (s *Store) SetSyncedAt(beanID string, t time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.ensureBean(beanID)
	utc := t.UTC()
	s.data.Beans[beanID].ClickUp.SyncedAt = &utc
}

// Clear removes all sync state for a bean.
func (s *Store) Clear(beanID string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	delete(s.data.Beans, beanID)
}

// GetAllBeans returns a copy of all bean sync states.
func (s *Store) GetAllBeans() map[string]*BeanSync {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make(map[string]*BeanSync, len(s.data.Beans))
	maps.Copy(result, s.data.Beans)
	return result
}

// Save writes the sync state to disk atomically (temp file + rename).
func (s *Store) Save() error {
	s.mu.RLock()
	data, err := json.MarshalIndent(s.data, "", "  ")
	s.mu.RUnlock()

	if err != nil {
		return fmt.Errorf("marshaling sync state: %w", err)
	}

	// Write to temp file first
	dir := filepath.Dir(s.filePath)
	tmpFile, err := os.CreateTemp(dir, ".sync-*.json.tmp")
	if err != nil {
		return fmt.Errorf("creating temp file: %w", err)
	}
	tmpPath := tmpFile.Name()

	// Clean up temp file on any error
	defer func() {
		if tmpPath != "" {
			_ = os.Remove(tmpPath)
		}
	}()

	if _, err := tmpFile.Write(data); err != nil {
		_ = tmpFile.Close()
		return fmt.Errorf("writing temp file: %w", err)
	}
	if err := tmpFile.Close(); err != nil {
		return fmt.Errorf("closing temp file: %w", err)
	}

	// Atomic rename
	if err := os.Rename(tmpPath, s.filePath); err != nil {
		return fmt.Errorf("renaming temp file: %w", err)
	}

	tmpPath = "" // Prevent cleanup since rename succeeded
	return nil
}

// ensureBean ensures the bean entry and its ClickUp sync struct exist.
// Must be called with s.mu held for writing.
func (s *Store) ensureBean(beanID string) {
	if s.data.Beans[beanID] == nil {
		s.data.Beans[beanID] = &BeanSync{}
	}
	if s.data.Beans[beanID].ClickUp == nil {
		s.data.Beans[beanID].ClickUp = &ClickUpSync{}
	}
}
