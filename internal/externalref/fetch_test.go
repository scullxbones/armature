package externalref

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFetchGitHubIssueMissingToken_REQ_HOSTREF_S1_T4(t *testing.T) {
	t.Parallel()

	issue := mustParseIssue(t, "https://github.com/octocat/hello-world/issues/1")
	rt := &recordingTransport{t: t}
	got, err := fetchGitHubIssue(context.Background(), issue, "", rt)
	require.ErrorIs(t, err, ErrMissingGitHubToken)
	assert.Equal(t, PlanText{}, got)
	assert.Equal(t, 0, rt.calls)
}

func TestFetchGitHubIssueReadsTitleAndBody_REQ_HOSTREF_S1_T4(t *testing.T) {
	t.Parallel()

	issue := mustParseIssue(t, "https://github.com/OctoCat/Hello-World/issues/01")
	rt := jsonIssueTransport(t, http.StatusOK, map[string]any{
		"title":    "Issue title",
		"body":     "the plan",
		"html_url": "https://github.com/octocat/hello-world/issues/1",
		"state":    "open",
	}, func(t *testing.T, req *http.Request) {
		t.Helper()
		assert.Equal(t, http.MethodGet, req.Method)
		assert.Equal(t, "https://api.github.com/repos/octocat/hello-world/issues/1", req.URL.String())
		assert.Equal(t, "Bearer test-token", req.Header.Get("Authorization"))
		assert.Nil(t, req.Body)
	})

	got, err := fetchGitHubIssue(context.Background(), issue, "test-token", rt)
	require.NoError(t, err)
	assert.Equal(t, PlanText{
		URL:   "https://github.com/octocat/hello-world/issues/1",
		Title: "Issue title",
		Body:  "the plan",
	}, got)
	assert.Equal(t, 1, rt.calls)
}

func TestFetchGitHubIssueNullBody_REQ_HOSTREF_S1_T4(t *testing.T) {
	t.Parallel()

	issue := mustParseIssue(t, "https://github.com/octocat/hello-world/issues/2")
	rt := jsonIssueTransport(t, http.StatusOK, map[string]any{
		"title": "No body",
		"body":  nil,
	}, nil)

	got, err := fetchGitHubIssue(context.Background(), issue, "test-token", rt)
	require.NoError(t, err)
	assert.Equal(t, PlanText{
		URL:   "https://github.com/octocat/hello-world/issues/2",
		Title: "No body",
		Body:  "",
	}, got)
}

func TestFetchGitHubIssueHTTPStatus_REQ_HOSTREF_S1_T4(t *testing.T) {
	t.Parallel()

	issue := mustParseIssue(t, "https://github.com/octocat/hello-world/issues/3")
	tests := []struct {
		name string
		code int
	}{
		{name: "unauthorized", code: http.StatusUnauthorized},
		{name: "not found", code: http.StatusNotFound},
		{name: "created is not success", code: http.StatusCreated},
		{name: "redirect is not followed as success", code: http.StatusFound},
		{name: "server error", code: http.StatusInternalServerError},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			rt := jsonIssueTransport(t, tc.code, map[string]any{"title": "x", "body": "y"}, nil)
			got, err := fetchGitHubIssue(context.Background(), issue, "test-token", rt)
			require.Error(t, err)
			require.NotErrorIs(t, err, ErrMissingGitHubToken)
			assert.Contains(t, err.Error(), "HTTP")
			assert.NotContains(t, err.Error(), "test-token")
			assert.Equal(t, PlanText{}, got)
			assert.Equal(t, 1, rt.calls)
		})
	}
}

func TestFetchGitHubIssueInvalidJSON_REQ_HOSTREF_S1_T4(t *testing.T) {
	t.Parallel()

	issue := mustParseIssue(t, "https://github.com/octocat/hello-world/issues/4")
	rt := &recordingTransport{
		t: t,
		handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			assert.Equal(t, http.MethodGet, r.Method)
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, err := w.Write([]byte("{"))
			assert.NoError(t, err)
		}),
	}
	got, err := fetchGitHubIssue(context.Background(), issue, "test-token", rt)
	require.Error(t, err)
	assert.Equal(t, PlanText{}, got)
	assert.NotContains(t, err.Error(), "test-token")
}

func TestFetchGitHubIssueRejectsNonIssue_REQ_HOSTREF_S1_T4(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		issue GitHubIssue
	}{
		{name: "zero value", issue: GitHubIssue{}},
		{name: "forged host", issue: GitHubIssue{canonical: "https://evil.example/octocat/hello-world/issues/1"}},
		{name: "forged api host", issue: GitHubIssue{canonical: "https://api.github.com/repos/octocat/hello-world/issues/1"}},
		{name: "query on stored canonical", issue: GitHubIssue{canonical: "https://github.com/octocat/hello-world/issues/1?x=1"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			rt := &recordingTransport{t: t}
			got, err := fetchGitHubIssue(context.Background(), tc.issue, "test-token", rt)
			require.ErrorIs(t, err, ErrInvalidGitHubIssueURL)
			assert.Equal(t, PlanText{}, got)
			assert.Equal(t, 0, rt.calls)
		})
	}
}

func TestFetchGitHubIssueCanceledContext_REQ_HOSTREF_S1_T4(t *testing.T) {
	t.Parallel()

	issue := mustParseIssue(t, "https://github.com/octocat/hello-world/issues/5")
	rt := &recordingTransport{t: t}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	got, err := fetchGitHubIssue(ctx, issue, "test-token", rt)
	require.Error(t, err)
	require.ErrorIs(t, err, context.Canceled)
	assert.Equal(t, PlanText{}, got)
	assert.Equal(t, 0, rt.calls)
}

func TestFetchGitHubIssueTokenFromEnv_REQ_HOSTREF_S1_T4(t *testing.T) {
	t.Parallel()

	issue := mustParseIssue(t, "https://github.com/octocat/hello-world/issues/1")

	t.Run("nil lookup", func(t *testing.T) {
		t.Parallel()
		got, err := FetchGitHubIssue(context.Background(), issue, nil)
		require.ErrorIs(t, err, ErrMissingGitHubToken)
		assert.Equal(t, PlanText{}, got)
	})

	t.Run("unset", func(t *testing.T) {
		t.Parallel()
		got, err := FetchGitHubIssue(context.Background(), issue, func(string) (string, bool) {
			return "", false
		})
		require.ErrorIs(t, err, ErrMissingGitHubToken)
		assert.Equal(t, PlanText{}, got)
	})

	t.Run("empty", func(t *testing.T) {
		t.Parallel()
		got, err := FetchGitHubIssue(context.Background(), issue, func(string) (string, bool) {
			return "", true
		})
		require.ErrorIs(t, err, ErrMissingGitHubToken)
		assert.Equal(t, PlanText{}, got)
	})

	t.Run("looks up GITHUB_TOKEN", func(t *testing.T) {
		t.Parallel()
		var key string
		got, err := FetchGitHubIssue(context.Background(), GitHubIssue{}, func(k string) (string, bool) {
			key = k
			return "test-token", true
		})
		require.ErrorIs(t, err, ErrInvalidGitHubIssueURL)
		assert.Equal(t, "GITHUB_TOKEN", key)
		assert.Equal(t, PlanText{}, got)
	})
}

func mustParseIssue(t *testing.T, raw string) GitHubIssue {
	t.Helper()
	issue, err := ParseGitHubIssueURL(raw)
	require.NoError(t, err)
	return issue
}

type recordingTransport struct {
	t       *testing.T
	handler http.Handler
	calls   int
}

func (r *recordingTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if err := req.Context().Err(); err != nil {
		return nil, err
	}
	r.calls++
	if r.handler == nil {
		r.t.Errorf("unexpected HTTP %s %s", req.Method, req.URL)
		return nil, errors.New("unexpected HTTP call")
	}
	rec := httptest.NewRecorder()
	r.handler.ServeHTTP(rec, req)
	return rec.Result(), nil
}

func jsonIssueTransport(t *testing.T, status int, payload map[string]any, inspect func(*testing.T, *http.Request)) *recordingTransport {
	t.Helper()
	return &recordingTransport{
		t: t,
		handler: http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			if inspect != nil {
				inspect(t, req)
			}
			body, err := json.Marshal(payload)
			require.NoError(t, err)
			w.Header().Set("Content-Type", "application/json")
			if status >= http.StatusMultipleChoices && status < http.StatusBadRequest {
				w.Header().Set("Location", "https://evil.example/stolen")
			}
			w.WriteHeader(status)
			_, err = w.Write(body)
			assert.NoError(t, err)
		}),
	}
}
