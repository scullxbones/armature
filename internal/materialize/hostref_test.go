package materialize

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/scullxbones/armature/internal/ops"
)

const (
	hostURLA = "https://github.com/scullxbones/armature/issues/1"
	hostURLB = "https://github.com/scullxbones/armature/issues/2"
)

func TestCurrentHostRef_REQ_HOSTREF_S1_T2(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		refs []HostRef
		ok   bool
		want HostRef
	}{
		{name: "empty history has no current"},
		{
			name: "single live name is current",
			refs: []HostRef{{URL: hostURLA, Role: "engineering", At: 10, WorkerID: "w1"}},
			ok:   true,
			want: HostRef{URL: hostURLA, Role: "engineering", At: 10, WorkerID: "w1"},
		},
		{
			name: "latest live name is current",
			refs: []HostRef{
				{URL: hostURLA, Role: "support", At: 10, WorkerID: "w1"},
				{URL: hostURLB, Role: "engineering", At: 20, WorkerID: "w2"},
			},
			ok:   true,
			want: HostRef{URL: hostURLB, Role: "engineering", At: 20, WorkerID: "w2"},
		},
		{
			name: "withdraw of current leaves the previous live name",
			refs: []HostRef{
				{URL: hostURLA, Role: "support", At: 10, WorkerID: "w1"},
				{URL: hostURLB, Role: "engineering", At: 20, WorkerID: "w2"},
				{URL: hostURLB, At: 30, WorkerID: "w2", Withdrawn: true},
			},
			ok:   true,
			want: HostRef{URL: hostURLA, Role: "support", At: 10, WorkerID: "w1"},
		},
		{
			name: "withdraw of a URL that is not live leaves current",
			refs: []HostRef{
				{URL: hostURLA, Role: "support", At: 10, WorkerID: "w1"},
				{URL: hostURLB, At: 20, WorkerID: "w1", Withdrawn: true},
			},
			ok:   true,
			want: HostRef{URL: hostURLA, Role: "support", At: 10, WorkerID: "w1"},
		},
		{
			name: "re-adding a withdrawn URL makes it current",
			refs: []HostRef{
				{URL: hostURLA, Role: "support", At: 10, WorkerID: "w1"},
				{URL: hostURLA, At: 20, WorkerID: "w1", Withdrawn: true},
				{URL: hostURLA, Role: "engineering", At: 30, WorkerID: "w2"},
			},
			ok:   true,
			want: HostRef{URL: hostURLA, Role: "engineering", At: 30, WorkerID: "w2"},
		},
		{
			name: "recording the same URL after a different name makes it current",
			refs: []HostRef{
				{URL: hostURLA, Role: "support", At: 10, WorkerID: "w1"},
				{URL: hostURLB, Role: "engineering", At: 20, WorkerID: "w1"},
				{URL: hostURLA, Role: "support", At: 30, WorkerID: "w1"},
			},
			ok:   true,
			want: HostRef{URL: hostURLA, Role: "support", At: 30, WorkerID: "w1"},
		},
		{
			name: "later live event for the current URL replaces At and WorkerID",
			refs: []HostRef{
				{URL: hostURLA, Role: "support", At: 10, WorkerID: "w1"},
				{URL: hostURLA, Role: "support", At: 20, WorkerID: "w2"},
			},
			ok:   true,
			want: HostRef{URL: hostURLA, Role: "support", At: 20, WorkerID: "w2"},
		},
		{
			name: "all withdrawn means no current",
			refs: []HostRef{
				{URL: hostURLA, Role: "support", At: 10, WorkerID: "w1"},
				{URL: hostURLA, At: 20, WorkerID: "w1", Withdrawn: true},
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, ok := CurrentHostRef(tc.refs)
			assert.Equal(t, tc.ok, ok)
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestApplyExternalRefAlwaysAppends_REQ_HOSTREF_S1_T2(t *testing.T) {
	t.Parallel()
	state := NewState()
	require.NoError(t, state.ApplyOp(ops.Op{
		Type: ops.OpCreate, TargetID: "task-01", Timestamp: 100, WorkerID: "w1",
		Payload: ops.Payload{Title: "T", NodeType: "task"},
	}))

	live := ops.Op{
		Type: ops.OpExternalRef, TargetID: "task-01", Timestamp: 200, WorkerID: "w1",
		Payload: ops.Payload{ExternalRef: hostURLA, ExternalRole: "engineering"},
	}
	require.NoError(t, state.ApplyOp(live))
	live.Timestamp = 300
	require.NoError(t, state.ApplyOp(live))

	issue := state.Issues["task-01"]
	require.Len(t, issue.HostRefs, 2)
	assert.Equal(t, []HostRef{
		{URL: hostURLA, Role: "engineering", At: 200, WorkerID: "w1"},
		{URL: hostURLA, Role: "engineering", At: 300, WorkerID: "w1"},
	}, issue.HostRefs)
	assert.Equal(t, int64(300), issue.Updated)

	got, ok := CurrentHostRef(issue.HostRefs)
	require.True(t, ok)
	assert.Equal(t, hostURLA, got.URL)
	assert.Equal(t, "engineering", got.Role)
}

func TestApplyExternalRefWithdrawAndReadd_REQ_HOSTREF_S1_T2(t *testing.T) {
	t.Parallel()
	state := NewState()
	require.NoError(t, state.ApplyOp(ops.Op{
		Type: ops.OpCreate, TargetID: "task-01", Timestamp: 100, WorkerID: "w1",
		Payload: ops.Payload{Title: "T", NodeType: "task"},
	}))
	require.NoError(t, state.ApplyOp(ops.Op{
		Type: ops.OpExternalRef, TargetID: "task-01", Timestamp: 200, WorkerID: "w1",
		Payload: ops.Payload{ExternalRef: hostURLA, ExternalRole: "support"},
	}))
	require.NoError(t, state.ApplyOp(ops.Op{
		Type: ops.OpExternalRef, TargetID: "task-01", Timestamp: 300, WorkerID: "w2",
		Payload: ops.Payload{WithdrawExternalRef: hostURLA},
	}))
	require.NoError(t, state.ApplyOp(ops.Op{
		Type: ops.OpExternalRef, TargetID: "task-01", Timestamp: 400, WorkerID: "w2",
		Payload: ops.Payload{ExternalRef: hostURLA, ExternalRole: "engineering"},
	}))

	issue := state.Issues["task-01"]
	require.Len(t, issue.HostRefs, 3)
	assert.Equal(t, HostRef{URL: hostURLA, At: 300, WorkerID: "w2", Withdrawn: true}, issue.HostRefs[1])
	got, ok := CurrentHostRef(issue.HostRefs)
	require.True(t, ok)
	assert.Equal(t, HostRef{URL: hostURLA, Role: "engineering", At: 400, WorkerID: "w2"}, got)
}

func TestApplyExternalRefMissingTargetIgnored_REQ_HOSTREF_S1_T2(t *testing.T) {
	t.Parallel()
	assert.Equal(t, onMissingIgnore, opHandlers()[ops.OpExternalRef].missingTarget)
	err := NewState().ApplyOp(ops.Op{
		Type: ops.OpExternalRef, TargetID: "missing-01", Timestamp: 200, WorkerID: "w1",
		Payload: ops.Payload{ExternalRef: hostURLA},
	})
	assert.NoError(t, err)
}

func TestExternalRefHandlerRegistered_REQ_HOSTREF_S1_T2(t *testing.T) {
	t.Parallel()
	assert.Contains(t, RegisteredOpTypes(), ops.OpExternalRef)
}

func TestBuildIndexStoresCurrentAndLiveHostURLs_REQ_HOSTREF_S1_T2(t *testing.T) {
	t.Parallel()
	state := NewState()
	require.NoError(t, state.ApplyOp(ops.Op{
		Type: ops.OpCreate, TargetID: "task-01", Timestamp: 100, WorkerID: "w1",
		Payload: ops.Payload{Title: "T", NodeType: "task"},
	}))
	require.NoError(t, state.ApplyOp(ops.Op{
		Type: ops.OpExternalRef, TargetID: "task-01", Timestamp: 200, WorkerID: "w1",
		Payload: ops.Payload{ExternalRef: hostURLA, ExternalRole: "support"},
	}))
	require.NoError(t, state.ApplyOp(ops.Op{
		Type: ops.OpExternalRef, TargetID: "task-01", Timestamp: 300, WorkerID: "w1",
		Payload: ops.Payload{ExternalRef: hostURLB, ExternalRole: "engineering"},
	}))

	entry := state.BuildIndex()["task-01"]
	assert.Equal(t, hostURLB, entry.CurrentHostURL)
	assert.Equal(t, []string{hostURLA, hostURLB}, entry.LiveHostURLs)

	indexPath := filepath.Join(t.TempDir(), "index.json")
	require.NoError(t, WriteIndex(indexPath, state.BuildIndex()))
	loaded, err := LoadIndex(indexPath)
	require.NoError(t, err)
	assert.Equal(t, hostURLB, loaded["task-01"].CurrentHostURL)
	assert.Equal(t, []string{hostURLA, hostURLB}, loaded["task-01"].LiveHostURLs)

	require.NoError(t, state.ApplyOp(ops.Op{
		Type: ops.OpExternalRef, TargetID: "task-01", Timestamp: 400, WorkerID: "w1",
		Payload: ops.Payload{WithdrawExternalRef: hostURLA},
	}))
	entry = state.BuildIndex()["task-01"]
	assert.Equal(t, hostURLB, entry.CurrentHostURL)
	assert.Equal(t, []string{hostURLB}, entry.LiveHostURLs)

	require.NoError(t, state.ApplyOp(ops.Op{
		Type: ops.OpExternalRef, TargetID: "task-01", Timestamp: 500, WorkerID: "w1",
		Payload: ops.Payload{WithdrawExternalRef: hostURLB},
	}))
	entry = state.BuildIndex()["task-01"]
	assert.Empty(t, entry.CurrentHostURL)
	assert.Empty(t, entry.LiveHostURLs)
}

func TestIssueHostRefsRoundTrip_REQ_HOSTREF_S1_T2(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	issue := Issue{
		ID:     "task-01",
		Type:   "task",
		Status: "open",
		Title:  "T",
		HostRefs: []HostRef{
			{URL: hostURLA, Role: "engineering", At: 200, WorkerID: "w1"},
			{URL: hostURLA, At: 300, WorkerID: "w1", Withdrawn: true},
		},
	}
	require.NoError(t, WriteIssue(dir, issue))
	loaded, err := LoadIssue(filepath.Join(dir, "task-01.json"))
	require.NoError(t, err)
	assert.Equal(t, issue.HostRefs, loaded.HostRefs)
}
