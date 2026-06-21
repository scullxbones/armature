package issuetype

import (
	"testing"
)

func TestIsValid(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		t        string
		expected bool
	}{
		{"epic", "epic", true},
		{"story", "story", true},
		{"feature", "feature", true},
		{"task", "task", true},
		{"bug", "bug", true},
		{"invalid type", "invalid", false},
		{"empty string", "", false},
		{"unknown", "spike", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := IsValid(tt.t)
			if got != tt.expected {
				t.Errorf("IsValid(%q) = %v, want %v", tt.t, got, tt.expected)
			}
		})
	}
}

func TestIsLegalHierarchy(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		parent   string
		child    string
		expected bool
	}{
		// Epic can contain story, feature, task, bug
		{"epic->story", "epic", "story", true},
		{"epic->feature", "epic", "feature", true},
		{"epic->task", "epic", "task", true},
		{"epic->bug", "epic", "bug", true},
		{"epic->epic", "epic", "epic", false},

		// Story can contain task, bug
		{"story->task", "story", "task", true},
		{"story->bug", "story", "bug", true},
		{"story->story", "story", "story", false},
		{"story->epic", "story", "epic", false},
		{"story->feature", "story", "feature", false},

		// Feature can contain task, bug
		{"feature->task", "feature", "task", true},
		{"feature->bug", "feature", "bug", true},
		{"feature->story", "feature", "story", false},
		{"feature->epic", "feature", "epic", false},
		{"feature->feature", "feature", "feature", false},

		// Task cannot contain anything
		{"task->task", "task", "task", false},
		{"task->epic", "task", "epic", false},
		{"task->story", "task", "story", false},
		{"task->bug", "task", "bug", false},

		// Bug cannot contain anything
		{"bug->bug", "bug", "bug", false},
		{"bug->epic", "bug", "epic", false},
		{"bug->task", "bug", "task", false},

		// Invalid parents
		{"invalid->task", "invalid", "task", false},
		{"epic->invalid", "epic", "invalid", false},
		{"invalid->invalid", "invalid", "invalid", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := IsLegalHierarchy(tt.parent, tt.child)
			if got != tt.expected {
				t.Errorf("IsLegalHierarchy(%q, %q) = %v, want %v", tt.parent, tt.child, got, tt.expected)
			}
		})
	}
}

func TestIsWorkable(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		t        string
		expected bool
	}{
		// Only task is workable (executable) - can be claimed and executed
		{"task is workable", "task", true},
		{"epic not workable", "epic", false},
		{"story not workable", "story", false},
		{"feature not workable", "feature", false},
		{"bug not workable", "bug", false},
		{"invalid not workable", "invalid", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := IsWorkable(tt.t)
			if got != tt.expected {
				t.Errorf("IsWorkable(%q) = %v, want %v", tt.t, got, tt.expected)
			}
		})
	}
}

func TestRequiresAcceptance(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		t        string
		expected bool
	}{
		// Epic, story, and task require acceptance criteria
		{"epic requires acceptance", "epic", true},
		{"story requires acceptance", "story", true},
		{"task requires acceptance", "task", true},
		{"feature does not require acceptance", "feature", false},
		{"bug does not require acceptance", "bug", false},
		{"invalid does not require acceptance", "invalid", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := RequiresAcceptance(tt.t)
			if got != tt.expected {
				t.Errorf("RequiresAcceptance(%q) = %v, want %v", tt.t, got, tt.expected)
			}
		})
	}
}

func TestAll(t *testing.T) {
	t.Parallel()
	result := All()

	// Check that all expected types are present
	expected := map[string]bool{
		"epic":    true,
		"story":   true,
		"feature": true,
		"task":    true,
		"bug":     true,
	}

	if len(result) != len(expected) {
		t.Errorf("All() returned %d types, want %d", len(result), len(expected))
	}

	for _, typeStr := range result {
		if !expected[typeStr] {
			t.Errorf("All() contains unexpected type %q", typeStr)
		}
	}

	for expectedType := range expected {
		found := false
		for _, typeStr := range result {
			if typeStr == expectedType {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("All() missing expected type %q", expectedType)
		}
	}
}
