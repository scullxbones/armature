package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/scullxbones/armature/internal/context"
	"github.com/scullxbones/armature/internal/materialize"
)

func TestContextIssueFactsCopiesHostRefs_REQ_HOSTREF_S1_T8(t *testing.T) {
	t.Parallel()

	issue := &materialize.Issue{
		ID: "HOST-1",
		HostRefs: []materialize.HostRef{
			{URL: "https://github.com/scullxbones/armature/issues/1", Role: "support", At: 10, WorkerID: "w1"},
			{URL: "https://github.com/scullxbones/armature/issues/1", At: 20, WorkerID: "w1", Withdrawn: true},
		},
	}

	facts := contextIssueFacts(issue)
	require.Len(t, facts.HostRefs, 2)
	assert.Equal(t, []context.HostRefFact{
		{URL: "https://github.com/scullxbones/armature/issues/1", Role: "support"},
		{URL: "https://github.com/scullxbones/armature/issues/1", Withdrawn: true},
	}, facts.HostRefs)

	facts.HostRefs[0].Role = "mutated"
	assert.Equal(t, "support", issue.HostRefs[0].Role, "context facts must copy host refs, not alias them")
}
