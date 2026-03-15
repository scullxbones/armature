package validate

import (
	"testing"

	"github.com/scullxbones/trellis/internal/materialize"
	"github.com/stretchr/testify/assert"
)

func makeState(issues ...*materialize.Issue) *materialize.State {
	s := materialize.NewState()
	for _, issue := range issues {
		s.Issues[issue.ID] = issue
	}
	return s
}

func TestValidate_Clean(t *testing.T) {
	state := makeState(
		&materialize.Issue{ID: "A", BlockedBy: []string{}, Children: []string{}},
		&materialize.Issue{ID: "B", BlockedBy: []string{}, Children: []string{}},
	)
	result := Validate(state)
	assert.True(t, result.OK)
	assert.Nil(t, result.Errors)
}

func TestValidate_OrphanedChild(t *testing.T) {
	state := makeState(
		&materialize.Issue{ID: "A", Parent: "nonexistent", BlockedBy: []string{}, Children: []string{}},
	)
	result := Validate(state)
	assert.False(t, result.OK)
	assert.Len(t, result.Errors, 1)
	assert.Contains(t, result.Errors[0], "unknown parent")
}

func TestValidate_CircularDep(t *testing.T) {
	state := makeState(
		&materialize.Issue{ID: "A", BlockedBy: []string{"B"}, Children: []string{}},
		&materialize.Issue{ID: "B", BlockedBy: []string{"A"}, Children: []string{}},
	)
	result := Validate(state)
	assert.False(t, result.OK)
	// At least one circular dependency error should be present
	found := false
	for _, e := range result.Errors {
		if e == "issue A has circular dependency" || e == "issue B has circular dependency" {
			found = true
			break
		}
	}
	assert.True(t, found, "expected circular dependency error, got: %v", result.Errors)
}

func TestValidate_UnknownBlocker(t *testing.T) {
	state := makeState(
		&materialize.Issue{ID: "A", BlockedBy: []string{"ghost"}, Children: []string{}},
	)
	result := Validate(state)
	assert.False(t, result.OK)
	assert.Len(t, result.Errors, 1)
	assert.Contains(t, result.Errors[0], "unknown blocker")
}
