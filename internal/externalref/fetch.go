package externalref

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
)

var ErrMissingGitHubToken = errors.New("GITHUB_TOKEN is missing")

type PlanText struct {
	URL   string
	Title string
	Body  string
}

func FetchGitHubIssue(ctx context.Context, issue GitHubIssue, lookup func(string) (string, bool)) (PlanText, error) {
	return fetchGitHubIssue(ctx, issue, gitHubToken(lookup), http.DefaultTransport)
}

func fetchGitHubIssue(ctx context.Context, issue GitHubIssue, token string, rt http.RoundTripper) (PlanText, error) {
	if token == "" {
		return PlanText{}, ErrMissingGitHubToken
	}
	apiURL, ok := githubAPIURL(issue)
	if !ok {
		return PlanText{}, ErrInvalidGitHubIssueURL
	}
	resp, err := gitHubDo(ctx, "fetch", http.MethodGet, apiURL, token, nil, rt, http.StatusOK)
	if err != nil {
		return PlanText{}, err
	}
	defer closeGitHubBody(resp)
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
