package main

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	armerrors "github.com/scullxbones/armature/internal/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestClassifyGitPushFailure_AuthNonFFOther_REQ_OPS_PUBLISH(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name   string
		stderr string
		want   opsPublishClass
	}{
		{
			name: "github https 403",
			stderr: `remote: Permission to scullxbones/armature.git denied to scullxbones.
fatal: unable to access 'https://github.com/scullxbones/armature.git/': The requested URL returned error: 403`,
			want: opsPublishClassAuth,
		},
		{
			name:   "authentication failed",
			stderr: "fatal: Authentication failed for 'https://github.com/example/repo.git/'",
			want:   opsPublishClassAuth,
		},
		{
			name:   "could not read Username",
			stderr: "fatal: could not read Username for 'https://github.com': terminal prompts disabled",
			want:   opsPublishClassAuth,
		},
		{
			name: "ssh publickey denied",
			stderr: `git@github.com: Permission denied (publickey).
fatal: Could not read from remote repository.`,
			want: opsPublishClassAuth,
		},
		{
			name: "non-fast-forward rejected",
			stderr: `! [rejected]        _armature -> _armature (non-fast-forward)
error: failed to push some refs to 'https://github.com/example/repo.git'
hint: Updates were rejected because the tip of your current branch is behind
hint: its remote counterpart. If you want to integrate the remote changes,
hint: use 'git pull' before pushing again.`,
			want: opsPublishClassNonFF,
		},
		{
			name: "fetch first",
			stderr: `! [rejected]        _armature -> _armature (fetch first)
error: failed to push some refs to 'origin'
hint: Updates were rejected because the remote contains work that you do
hint: not have locally.`,
			want: opsPublishClassNonFF,
		},
		{
			name:   "updates were rejected because the tip",
			stderr: "hint: Updates were rejected because the tip of your current branch is behind",
			want:   opsPublishClassNonFF,
		},
		{
			name: "pre-receive hook declined is other not non-ff",
			stderr: `! [remote rejected] _armature -> _armature (pre-receive hook declined)
error: failed to push some refs to 'https://github.com/example/repo.git'`,
			want: opsPublishClassOther,
		},
		{
			name: "repo rule protected branch rejection is other not non-ff",
			stderr: `remote: error: GH006: Protected branch update failed for refs/heads/_armature.
! [remote rejected] _armature -> _armature (push declined due to repository rule)`,
			want: opsPublishClassOther,
		},
		{
			name:   "bare rejected without non-ff diagnostic is other",
			stderr: "error: failed to push some refs to 'origin' (rejected)",
			want:   opsPublishClassOther,
		},
		{
			name:   "unknown host falls to other",
			stderr: "fatal: unable to access 'https://github.com/example/repo.git/': Could not resolve host: github.com",
			want:   opsPublishClassOther,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := classifyGitPushFailure(tc.stderr)
			assert.Equal(t, tc.want, got)
			wrapped := classifyGitPushFailureErr(errors.New("git push origin _armature: exit status 1\n" + tc.stderr))
			assert.Equal(t, tc.want, wrapped)
		})
	}
}

func TestOpsPublishErrorCarriesClass_REQ_OPS_PUBLISH(t *testing.T) {
	t.Parallel()
	err := newOpsPublishError(errors.New("fatal: Authentication failed for 'https://github.com/x/y.git/'"))
	require.Error(t, err)
	assert.Equal(t, opsPublishClassAuth, err.class)
	assert.Contains(t, err.Error(), "class=auth")
	assert.Contains(t, err.Error(), "publish _armature")
	assert.True(t, isLocalArmatureTipPublishError(err))

	actions := nextActionsForOpsPublish(err)
	joined := strings.Join(actions, "\n")
	assert.Contains(t, joined, "Contents: Write")
	assert.NotContains(t, joined, "arm doctor")

	nonFF := newOpsPublishError(errors.New("! [rejected] _armature -> _armature (non-fast-forward)"))
	assert.Equal(t, opsPublishClassNonFF, nonFF.class)
	assert.Contains(t, nonFF.Error(), "class=non-fast-forward")
	nonFFActions := strings.Join(nextActionsForOpsPublish(nonFF), "\n")
	assert.Contains(t, nonFFActions, "arm push-ops")
	assert.Contains(t, nonFFActions, "rebase")
	assert.NotContains(t, nonFFActions, "arm doctor")

	other := newOpsPublishError(errors.New("fatal: Could not resolve host: github.com"))
	assert.Equal(t, opsPublishClassOther, other.class)
	assert.Contains(t, other.Error(), "class=other")
	otherActions := strings.Join(nextActionsForOpsPublish(other), "\n")
	assert.Contains(t, otherActions, "arm push-ops")
	assert.Contains(t, otherActions, "arm doctor")

	mapped := wrapOpsPublishFailure(codeClaim1, err)
	var cf *armerrors.CommandFailure
	require.True(t, errors.As(mapped, &cf))
	human := new(bytes.Buffer)
	renderCommandFailure(human, "human", cf)
	assert.Contains(t, human.String(), "Error [CLAIM-1]:")
	assert.Contains(t, human.String(), "class=auth")
	assert.Contains(t, human.String(), "Contents: Write")
	agent := new(bytes.Buffer)
	renderCommandFailure(agent, "agent", cf)
	assert.Contains(t, agent.String(), `"code":"CLAIM-1"`)
	assert.Contains(t, agent.String(), "class=auth")
}
