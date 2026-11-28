package externalref

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// CreatedIssue is the host ticket opened for andon / dogfood signals.
type CreatedIssue struct {
	Number int
	URL    string
}

// CreateGitHubIssue opens an issue on owner/repo using GITHUB_TOKEN from lookup.
func CreateGitHubIssue(
	ctx context.Context,
	owner, repo, title, body string,
	labels []string,
	lookup func(string) (string, bool),
) (CreatedIssue, error) {
	return createGitHubIssue(ctx, owner, repo, title, body, labels, gitHubToken(lookup), http.DefaultTransport)
}

func createGitHubIssue(
	ctx context.Context,
	owner, repo, title, body string,
	labels []string,
	token string,
	rt http.RoundTripper,
) (CreatedIssue, error) {
	if token == "" {
		return CreatedIssue{}, ErrMissingGitHubToken
	}
	owner = strings.TrimSpace(owner)
	repo = strings.TrimSpace(repo)
	if owner == "" || repo == "" {
		return CreatedIssue{}, fmt.Errorf("GitHub issue create: owner and repo are required")
	}
	if strings.TrimSpace(title) == "" {
		return CreatedIssue{}, fmt.Errorf("GitHub issue create: title is required")
	}
	apiURL := gitHubAPIOrigin + "/repos/" + owner + "/" + repo + "/issues"
	payload := map[string]any{
		"title": title,
		"body":  body,
	}
	if len(labels) > 0 {
		payload["labels"] = labels
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return CreatedIssue{}, fmt.Errorf("GitHub issue create: %w", err)
	}
	resp, err := gitHubDo(ctx, "create", http.MethodPost, apiURL, token, bytes.NewReader(raw), rt, http.StatusCreated)
	if err != nil {
		return CreatedIssue{}, err
	}
	defer closeGitHubBody(resp)
	var decoded struct {
		Number  int    `json:"number"`
		HTMLURL string `json:"html_url"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, maxGitHubIssueJSONBytes)).Decode(&decoded); err != nil {
		return CreatedIssue{}, fmt.Errorf("GitHub issue create: %w", err)
	}
	if decoded.Number <= 0 || decoded.HTMLURL == "" {
		return CreatedIssue{}, fmt.Errorf("GitHub issue create: missing number or html_url")
	}
	return CreatedIssue{Number: decoded.Number, URL: decoded.HTMLURL}, nil
}
