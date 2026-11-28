package externalref

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"time"
)

const (
	gitHubAPIOrigin         = "https://api.github.com"
	maxGitHubIssueJSONBytes = 1 << 20
	gitHubHTTPTimeout       = 30 * time.Second
	gitHubAccept            = "application/vnd.github+json"
)

func gitHubToken(lookup func(string) (string, bool)) string {
	if lookup == nil {
		return ""
	}
	token, _ := lookup("GITHUB_TOKEN")
	return token
}

func gitHubDo(ctx context.Context, verb, method, apiURL, token string, body io.Reader, rt http.RoundTripper, okStatus int) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, method, apiURL, body)
	if err != nil {
		return nil, fmt.Errorf("GitHub issue %s: %w", verb, err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", gitHubAccept)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	client := &http.Client{
		Timeout:   gitHubHTTPTimeout,
		Transport: rt,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("GitHub issue %s: %w", verb, err)
	}
	if resp.StatusCode != okStatus {
		_, drainErr := io.Copy(io.Discard, io.LimitReader(resp.Body, maxGitHubIssueJSONBytes))
		closeGitHubBody(resp)
		if drainErr != nil {
			return nil, fmt.Errorf("GitHub issue %s: HTTP %d: %w", verb, resp.StatusCode, drainErr)
		}
		return nil, fmt.Errorf("GitHub issue %s: HTTP %d", verb, resp.StatusCode)
	}
	return resp, nil
}

func closeGitHubBody(resp *http.Response) {
	swallowErr(resp.Body.Close())
}

func swallowErr(err error) { _ = err }
