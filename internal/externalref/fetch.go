package externalref

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

var ErrMissingGitHubToken = errors.New("GITHUB_TOKEN is missing")

const (
	gitHubAPIOrigin         = "https://api.github.com"
	maxGitHubIssueJSONBytes = 1 << 20
	gitHubIssueFetchTimeout = 30 * time.Second
)

type PlanText struct {
	URL   string
	Title string
	Body  string
}

func FetchGitHubIssue(ctx context.Context, issue GitHubIssue, lookup func(string) (string, bool)) (PlanText, error) {
	token := ""
	if lookup != nil {
		token, _ = lookup("GITHUB_TOKEN")
	}
	return fetchGitHubIssue(ctx, issue, token, http.DefaultTransport)
}

func fetchGitHubIssue(ctx context.Context, issue GitHubIssue, token string, rt http.RoundTripper) (PlanText, error) {
	if token == "" {
		return PlanText{}, ErrMissingGitHubToken
	}
	apiURL, ok := githubAPIURL(issue)
	if !ok {
		return PlanText{}, ErrInvalidGitHubIssueURL
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
	if err != nil {
		return PlanText{}, fmt.Errorf("GitHub issue fetch: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/vnd.github+json")
	client := &http.Client{
		Timeout:   gitHubIssueFetchTimeout,
		Transport: rt,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	resp, err := client.Do(req)
	if err != nil {
		return PlanText{}, fmt.Errorf("GitHub issue fetch: %w", err)
	}
	defer func() {
		_ = resp.Body.Close() //nolint:errcheck // close after read or drain is not actionable
	}()
	if resp.StatusCode != http.StatusOK {
		if _, drainErr := io.Copy(io.Discard, io.LimitReader(resp.Body, maxGitHubIssueJSONBytes)); drainErr != nil {
			return PlanText{}, fmt.Errorf("GitHub issue fetch: HTTP %d: %w", resp.StatusCode, drainErr)
		}
		return PlanText{}, fmt.Errorf("GitHub issue fetch: HTTP %d", resp.StatusCode)
	}
	var payload struct {
		Title string  `json:"title"`
		Body  *string `json:"body"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, maxGitHubIssueJSONBytes)).Decode(&payload); err != nil {
		return PlanText{}, fmt.Errorf("GitHub issue fetch: %w", err)
	}
	body := ""
	if payload.Body != nil {
		body = *payload.Body
	}
	return PlanText{URL: issue.URL(), Title: payload.Title, Body: body}, nil
}

func githubAPIURL(issue GitHubIssue) (string, bool) {
	canonical, ok := canonicalGitHubIssueURL(issue.canonical)
	if !ok || canonical != issue.canonical {
		return "", false
	}
	const prefix = "https://github.com/"
	rest, ok := strings.CutPrefix(canonical, prefix)
	if !ok || rest == "" {
		return "", false
	}
	return gitHubAPIOrigin + "/repos/" + rest, true
}
