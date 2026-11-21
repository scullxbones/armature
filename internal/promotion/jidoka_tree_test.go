package promotion

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestTreeRootAndMembers_REQ_482(t *testing.T) {
	t.Parallel()
	g := Graph{
		"epic-01":  {Children: []string{"story-01"}},
		"story-01": {Parent: "epic-01", Children: []string{"task-01", "task-02"}},
		"task-01":  {Parent: "story-01"},
		"task-02":  {Parent: "story-01"},
	}
	assert.Equal(t, "epic-01", treeRoot(g, "task-01"))
	members := treeMemberIDs(g, "epic-01")
	assert.Equal(t, []string{"epic-01", "story-01", "task-01", "task-02"}, members)
}
