package cmd

import (
	"strings"
	"testing"
)

func TestSanitizeUsername(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"john", "john"},
		{"John Doe", "john_doe"},
		{"john.doe", "john_doe"},
		{"John.Doe", "john_doe"},
		{"UPPERCASE", "uppercase"},
		{"user123", "user123"},
		{"user@example", "userexample"},
		{"user-name", "username"},
		{"user_name", "user_name"},
		{"  spaces  ", "__spaces__"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := sanitizeUsername(tt.input)
			if result != tt.expected {
				t.Errorf("sanitizeUsername(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestGenerateConfig(t *testing.T) {
	data := configTemplateData{
		ListID:   "123456789",
		ListName: "My Test List",
		Users: []userEntry{
			{Username: "alice", ID: 111, Email: "alice@example.com"},
			{Username: "bob", ID: 222, Email: "bob@example.com"},
		},
		Statuses: []string{"to do", "in progress", "complete"},
		CustomFields: []fieldEntry{
			{Name: "Bean ID", Type: "text", ID: "abc-123"},
			{Name: "Due Date", Type: "date", ID: "def-456"},
		},
	}

	result, err := generateConfig(data)
	if err != nil {
		t.Fatalf("generateConfig() error = %v", err)
	}

	// Check for required elements
	checks := []struct {
		name     string
		contains string
	}{
		{"list_id", `list_id: "123456789"`},
		{"list name comment", "# List: My Test List"},
		{"user alice", "# alice: 111  # alice@example.com"},
		{"user bob", "# bob: 222  # bob@example.com"},
		{"status to do", `- "to do"`},
		{"status in progress", `- "in progress"`},
		{"status complete", `- "complete"`},
		{"field Bean ID", `- "Bean ID" (text): abc-123`},
		{"field Due Date", `- "Due Date" (date): def-456`},
		{"status_mapping comment", "# status_mapping:"},
		{"custom_fields comment", "# custom_fields:"},
	}

	for _, c := range checks {
		t.Run(c.name, func(t *testing.T) {
			if !strings.Contains(result, c.contains) {
				t.Errorf("generateConfig() output missing %q\nGot:\n%s", c.contains, result)
			}
		})
	}
}

func TestGenerateConfig_NoOptionalData(t *testing.T) {
	data := configTemplateData{
		ListID:   "999",
		ListName: "Minimal List",
		Statuses: []string{"open", "closed"},
		// No users or custom fields
	}

	result, err := generateConfig(data)
	if err != nil {
		t.Fatalf("generateConfig() error = %v", err)
	}

	// Should have list_id
	if !strings.Contains(result, `list_id: "999"`) {
		t.Error("missing list_id")
	}

	// Should have statuses
	if !strings.Contains(result, `- "open"`) {
		t.Error("missing status 'open'")
	}

	// Should NOT have users section header (since no users)
	if strings.Contains(result, "Workspace members for @mention") {
		t.Error("should not have users section when no users provided")
	}

	// Should NOT have custom fields section (since no custom fields)
	if strings.Contains(result, "Custom fields: map bean fields") {
		t.Error("should not have custom fields section when no fields provided")
	}
}
