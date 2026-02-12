// Package beans provides a wrapper for the beans CLI.
package beans

import (
	"slices"
	"time"
)

// Standard bean types
const (
	TypeMilestone = "milestone"
	TypeEpic      = "epic"
	TypeFeature   = "feature"
	TypeBug       = "bug"
	TypeTask      = "task"
)

// Extension metadata constants
const (
	PluginClickUp = "clickup"
	ExtKeyTaskID  = "task_id"
	ExtKeySyncedAt = "synced_at"
)

// StandardTypes is the list of all standard bean types.
var StandardTypes = []string{TypeMilestone, TypeEpic, TypeFeature, TypeBug, TypeTask}

// IsStandardType returns true if the given type is a standard bean type.
func IsStandardType(t string) bool {
	return slices.Contains(StandardTypes, t)
}

// Bean represents a bean from the beans CLI JSON output.
type Bean struct {
	ID        string                        `json:"id"`
	Slug      string                        `json:"slug"`
	Path      string                        `json:"path"`
	Title     string                        `json:"title"`
	Status    string                        `json:"status"`
	Type      string                        `json:"type"`
	Priority  string                        `json:"priority,omitempty"`
	CreatedAt *time.Time                    `json:"created_at,omitempty"`
	UpdatedAt *time.Time                    `json:"updated_at,omitempty"`
	Body      string                        `json:"body,omitempty"`
	Parent    string                        `json:"parent,omitempty"`
	Blocking  []string                      `json:"blocking,omitempty"`
	Due       *string                        `json:"due,omitempty"`
	Tags      []string                      `json:"tags,omitempty"`
	Extensions map[string]map[string]any    `json:"extensions,omitempty"`
}

// GetExtensionString returns a string value from extension data.
// Returns empty string if not found.
func (b *Bean) GetExtensionString(name, key string) string {
	if b.Extensions == nil {
		return ""
	}
	extData, ok := b.Extensions[name]
	if !ok {
		return ""
	}
	val, ok := extData[key]
	if !ok {
		return ""
	}
	s, _ := val.(string)
	return s
}

// GetExtensionTime returns a time value from extension data.
// Expects the value to be an RFC3339 string. Returns nil if not found or unparseable.
func (b *Bean) GetExtensionTime(name, key string) *time.Time {
	s := b.GetExtensionString(name, key)
	if s == "" {
		return nil
	}
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return nil
	}
	return &t
}
