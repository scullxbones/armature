package externalref

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCreateGitHubIssue_REQ_482(t *testing.T) {
	t.Parallel()
	rt := &recordingTransport{
		t: t,
		handler: http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			assert.Equal(t, http.MethodPost, req.Method)
			assert.Equal(t, "https://api.github.com/repos/octocat/hello-world/issues", req.URL.String())
			assert.Equal(t, "Bearer test-token", req.Header.Get("Authorization"))
			body, err := io.ReadAll(req.Body)
			require.NoError(t, err)
			var payload map[string]any
			require.NoError(t, json.Unmarshal(body, &payload))
			assert.Equal(t, "andon title", payload["title"])
			assert.Equal(t, "body text", payload["body"])
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusCreated)
			_, err = w.Write([]byte(`{"number":42,"html_url":"https://github.com/octocat/hello-world/issues/42"}`))
			assert.NoError(t, err)
		}),
	}
	got, err := createGitHubIssue(context.Background(), "octocat", "hello-world", "andon title", "body text", nil, "test-token", rt)
	require.NoError(t, err)
	assert.Equal(t, CreatedIssue{Number: 42, URL: "https://github.com/octocat/hello-world/issues/42"}, got)
	assert.Equal(t, 1, rt.calls)
}

func TestCreateGitHubIssueMissingToken_REQ_482(t *testing.T) {
	t.Parallel()
	_, err := createGitHubIssue(context.Background(), "o", "r", "t", "b", nil, "", &recordingTransport{t: t})
	require.ErrorIs(t, err, ErrMissingGitHubToken)
}
