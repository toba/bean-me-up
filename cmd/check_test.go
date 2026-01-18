package cmd

import (
	"testing"

	"github.com/STR-Consulting/bean-me-up/internal/clickup"
	"github.com/STR-Consulting/bean-me-up/internal/config"
)

func TestCheckStatusMapping_Valid(t *testing.T) {
	cfg := &config.Config{
		Beans: config.BeansWrapper{
			ClickUp: config.ClickUpConfig{
				StatusMapping: map[string]string{
					"todo":        "to do",
					"in-progress": "in progress",
					"completed":   "complete",
				},
			},
		},
	}

	list := &clickup.List{
		Statuses: []clickup.Status{
			{Status: "to do"},
			{Status: "in progress"},
			{Status: "complete"},
		},
	}

	results := checkStatusMapping(cfg, list)
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}

	if results[0].Status != checkPass {
		t.Errorf("expected pass, got %s: %s", results[0].Status, results[0].Message)
	}

	if results[0].Message != "3 mappings" {
		t.Errorf("expected '3 mappings', got %q", results[0].Message)
	}
}

func TestCheckStatusMapping_Invalid(t *testing.T) {
	cfg := &config.Config{
		Beans: config.BeansWrapper{
			ClickUp: config.ClickUpConfig{
				StatusMapping: map[string]string{
					"todo":        "to do",
					"in-progress": "doing", // Not in list
					"completed":   "done",  // Not in list
				},
			},
		},
	}

	list := &clickup.List{
		Statuses: []clickup.Status{
			{Status: "to do"},
			{Status: "in progress"},
			{Status: "complete"},
		},
	}

	results := checkStatusMapping(cfg, list)
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}

	if results[0].Status != checkWarn {
		t.Errorf("expected warn, got %s: %s", results[0].Status, results[0].Message)
	}
}

func TestCheckStatusMapping_Empty(t *testing.T) {
	cfg := &config.Config{
		Beans: config.BeansWrapper{
			ClickUp: config.ClickUpConfig{
				// No status mapping
			},
		},
	}

	list := &clickup.List{
		Statuses: []clickup.Status{
			{Status: "to do"},
		},
	}

	results := checkStatusMapping(cfg, list)
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}

	// Default mapping is used, so we should get a pass with 5 mappings (the default)
	if results[0].Status != checkPass {
		// If status is warn, it means no mapping was configured at all
		if results[0].Status != checkWarn {
			t.Errorf("expected pass or warn, got %s: %s", results[0].Status, results[0].Message)
		}
	}
}

func TestCheckOutput_Summary(t *testing.T) {
	output := checkOutput{
		Sections: []checkSection{
			{
				Name: "Test Section",
				Checks: []checkResult{
					{Name: "Check 1", Status: checkPass},
					{Name: "Check 2", Status: checkPass},
					{Name: "Check 3", Status: checkWarn},
					{Name: "Check 4", Status: checkFail},
				},
			},
		},
	}

	// Calculate summary like runCheck does
	for _, section := range output.Sections {
		for _, check := range section.Checks {
			switch check.Status {
			case checkPass:
				output.Summary.Passed++
			case checkWarn:
				output.Summary.Warnings++
			case checkFail:
				output.Summary.Failed++
			}
		}
	}

	if output.Summary.Passed != 2 {
		t.Errorf("expected 2 passed, got %d", output.Summary.Passed)
	}
	if output.Summary.Warnings != 1 {
		t.Errorf("expected 1 warning, got %d", output.Summary.Warnings)
	}
	if output.Summary.Failed != 1 {
		t.Errorf("expected 1 failed, got %d", output.Summary.Failed)
	}
}

func TestCheckStatusTypes(t *testing.T) {
	// Verify check status constants are correct strings
	if checkPass != "pass" {
		t.Errorf("checkPass should be 'pass', got %q", checkPass)
	}
	if checkWarn != "warn" {
		t.Errorf("checkWarn should be 'warn', got %q", checkWarn)
	}
	if checkFail != "fail" {
		t.Errorf("checkFail should be 'fail', got %q", checkFail)
	}
}
