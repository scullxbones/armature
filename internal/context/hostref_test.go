package context

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/scullxbones/armature/internal/materialize"
)

const (
	hostRefURLA = "https://github.com/scullxbones/armature/issues/1"
	hostRefURLB = "https://github.com/scullxbones/armature/issues/2"
)

func TestRenderContextIncludesHostNames_REQ_HOSTREF_S1_T8(t *testing.T) {
	t.Parallel()

	state := materialize.NewState()
	state.Issues["HOST-1"] = &materialize.Issue{
		ID:     "HOST-1",
		Title:  "Issue with host names",
		Type:   "task",
		Status: "open",
		HostRefs: []materialize.HostRef{
			{URL: hostRefURLA, Role: "support", At: 10, WorkerID: "w1"},
			{URL: hostRefURLB, Role: "engineering", At: 20, WorkerID: "w2"},
			{URL: hostRefURLA, At: 30, WorkerID: "w1", Withdrawn: true},
			{URL: hostRefURLA, Role: "support", At: 40, WorkerID: "w3"},
		},
		Children:     []string{},
		BlockedBy:    []string{},
		Blocks:       []string{},
		DecisionRefs: []string{},
	}

	out, err := RenderAgent("HOST-1", factsFromState(state), Options{SkipTruncate: true})
	require.NoError(t, err)

	var mem workingMemory
	require.NoError(t, json.Unmarshal([]byte(out), &mem))

	var hostLayer *layer
	for i := range mem.Layers {
		if mem.Layers[i].Name == "host_refs" {
			hostLayer = &mem.Layers[i]
			break
		}
	}
	require.NotNil(t, hostLayer, "render-context must include a host_refs layer")
	content := hostLayer.Content

	assert.Contains(t, content, hostRefURLB)
	assert.Contains(t, content, "engineering")
	assert.Contains(t, content, hostRefURLA)
	assert.Contains(t, content, "support")
	assert.Contains(t, content, "current")

	bIdx := strings.Index(content, hostRefURLB)
	aIdx := strings.Index(content, hostRefURLA)
	require.GreaterOrEqual(t, bIdx, 0)
	require.GreaterOrEqual(t, aIdx, 0)
	assert.Less(t, bIdx, aIdx, "live names must appear in replay-derived order (B then re-added A)")

	currentIdx := strings.LastIndex(content, "current")
	assert.Greater(t, currentIdx, aIdx, "re-added A must be marked current")

	assert.NotContains(t, content, "body")
	assert.NotContains(t, strings.ToLower(content), "title:")
	assert.NotContains(t, out, "FetchGitHubIssue")
}

func TestRenderContextOmitsHostRefsLayerWhenNoneLive_REQ_HOSTREF_S1_T8(t *testing.T) {
	t.Parallel()

	state := materialize.NewState()
	state.Issues["HOST-0"] = &materialize.Issue{
		ID:     "HOST-0",
		Title:  "No live host names",
		Type:   "task",
		Status: "open",
		HostRefs: []materialize.HostRef{
			{URL: hostRefURLA, Role: "support", At: 10, WorkerID: "w1"},
			{URL: hostRefURLA, At: 20, WorkerID: "w1", Withdrawn: true},
		},
		Children:     []string{},
		BlockedBy:    []string{},
		Blocks:       []string{},
		DecisionRefs: []string{},
	}

	out, err := RenderAgent("HOST-0", factsFromState(state), Options{SkipTruncate: true})
	require.NoError(t, err)
	assert.NotContains(t, out, `"name": "host_refs"`)
	assert.NotContains(t, out, hostRefURLA)
}
