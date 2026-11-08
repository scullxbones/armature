package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// syncBuf is a mutex-backed buffer safe for concurrent Write (session) and
// String (test helper). Optionally signals once when needle appears.
type syncBuf struct {
	mu     sync.Mutex
	buf    bytes.Buffer
	needle string
	notify chan struct{}
	saw    bool
}

func newSyncBuf(needle string) *syncBuf {
	s := &syncBuf{needle: needle}
	if needle != "" {
		s.notify = make(chan struct{})
	}
	return s
}

func (s *syncBuf) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	n, err := s.buf.Write(p)
	if s.notify != nil && !s.saw && strings.Contains(s.buf.String(), s.needle) {
		s.saw = true
		close(s.notify)
	}
	return n, err
}

func (s *syncBuf) String() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.buf.String()
}

func runMCPToolCall(t *testing.T, repo string, args []string) (stdout string, exitCode int, err error) {
	t.Helper()
	text, code, _, err := runMCPToolCallRaw(t, repo, args)
	return text, code, err
}

func runMCPToolCallRaw(t *testing.T, repo string, args []string) (text string, exitCode int, raw string, err error) {
	t.Helper()
	argsJSON, marshalErr := json.Marshal(map[string]any{"args": args})
	require.NoError(t, marshalErr)
	paramsJSON, marshalErr := json.Marshal(map[string]any{
		"name":      "arm",
		"arguments": json.RawMessage(argsJSON),
	})
	require.NoError(t, marshalErr)
	req := fmt.Sprintf(
		`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}`+"\n"+
			`{"jsonrpc":"2.0","id":2,"method":"tools/call","params":%s}`+"\n",
		paramsJSON,
	)
	// Keep stdin open until the tools/call response is written so EOF does not
	// cancel an in-flight Runner (ADR 0024). Close afterward to end Serve.
	pr, pw := io.Pipe()
	out := newSyncBuf(`"id":2`)
	stderr := newSyncBuf("")
	done := make(chan int, 1)
	go func() {
		done <- Run(context.Background(), Invocation{
			Args:    []string{"mcp", "--repo", repo},
			Dir:     repo,
			Env:     OverlayEnv(OSEnv(), cliEnvFor(t)),
			Stdin:   pr,
			Stdout:  out,
			Stderr:  stderr,
			TempDir: t.TempDir(),
		})
	}()
	_, writeErr := io.WriteString(pw, req)
	require.NoError(t, writeErr)

	closePW := func() {
		t.Helper()
		require.NoError(t, pw.Close())
	}
	deadline := time.After(15 * time.Second)
	select {
	case code := <-done:
		closePW()
		return out.String(), 0, out.String(), fmt.Errorf("arm mcp exited %d before tools/call response: %s", code, stderr.String())
	case <-deadline:
		closePW()
		return out.String(), 0, out.String(), fmt.Errorf("timeout waiting for tools/call response: %s", stderr.String())
	case <-out.notify:
	}
	closePW()
	code := <-done
	raw = out.String()
	if code != 0 {
		return raw, 0, raw, fmt.Errorf("arm mcp exited %d: %s", code, stderr.String())
	}
	lines := nonEmptyMCPLines(raw)
	require.NotEmpty(t, lines)
	var resp struct {
		Result struct {
			Content []struct {
				Type string `json:"type"`
				Text string `json:"text"`
			} `json:"content"`
			StructuredContent struct {
				ExitCode int    `json:"exitCode"`
				Stdout   string `json:"stdout"`
				Stderr   string `json:"stderr"`
			} `json:"structuredContent"`
			IsError bool `json:"isError"`
		} `json:"result"`
		Error *struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	require.NoError(t, json.Unmarshal([]byte(lines[len(lines)-1]), &resp))
	if resp.Error != nil {
		return "", 0, raw, fmt.Errorf("rpc error: %s", resp.Error.Message)
	}
	text = resp.Result.StructuredContent.Stdout
	if text == "" && len(resp.Result.Content) > 0 {
		text = resp.Result.Content[0].Text
	}
	if text == "" && resp.Result.StructuredContent.Stderr != "" {
		text = resp.Result.StructuredContent.Stderr
	}
	return text, resp.Result.StructuredContent.ExitCode, raw, nil
}

func nonEmptyMCPLines(s string) []string {
	var out []string
	for _, line := range strings.Split(s, "\n") {
		if strings.TrimSpace(line) != "" {
			out = append(out, line)
		}
	}
	return out
}

func TestSessionArmRef_REQ_MCPSTDIO_S1_T5(t *testing.T) {
	repo := setupRefRepo(t, "mcp-s1-t5-ref")
	_, err := runTrls(t, repo, "worker-init")
	require.NoError(t, err)

	text, exitCode, err := runMCPToolCall(t, repo, []string{
		"ref", "mcp-s1-t5-ref", hostRefURLA, "--role", "engineering", "--format", "json",
	})
	require.NoError(t, err)
	assert.Equal(t, 0, exitCode)
	assert.Contains(t, text, hostRefURLA)
	assert.Contains(t, text, `"role":"engineering"`)
	got := externalRefOpsForIssue(t, repo, "mcp-s1-t5-ref")
	require.Len(t, got, 1)
	assert.Equal(t, hostRefURLA, got[0].Payload.ExternalRef)
}

func TestSessionArmRefRead_REQ_MCPSTDIO_S1_T5(t *testing.T) {
	repo := setupRepoWithTask(t)
	_, err := runTrls(t, repo, "worker-init")
	require.NoError(t, err)
	plantHostRef(t, repo, hostRefReadURLA, "engineering")
	withGitHubIssueTransport(t, http.StatusOK, map[string]any{
		"title": "MCP plan title",
		"body":  "MCP plan body",
	})
	setCLIEnv(t, "GITHUB_TOKEN", "test-token")

	text, exitCode, err := runMCPToolCall(t, repo, []string{
		"ref-read", "task-01", "--format", "human",
	})
	require.NoError(t, err)
	assert.Equal(t, 0, exitCode)
	assert.Equal(t, "MCP plan title\n\nMCP plan body\n", text)
}

func TestSessionArmListExternalRef_REQ_MCPSTDIO_S1_T5(t *testing.T) {
	repo := setupRepoWithStoryAndTask(t)
	_, err := runTrls(t, repo, "worker-init")
	require.NoError(t, err)
	appendExternalRefOp(t, repo, "task-01", listHostURLA, "engineering", "")

	text, exitCode, err := runMCPToolCall(t, repo, []string{
		"list", "--external-ref", listHostURLA, "--format", "json",
	})
	require.NoError(t, err)
	assert.Equal(t, 0, exitCode)
	assert.Contains(t, text, `"id":"task-01"`)
	assert.Contains(t, text, `"count":1`)
}

func TestSessionRegistersOnlyArmTool_REQ_MCPSTDIO_S1_T5(t *testing.T) {
	repo := setupRepoWithTask(t)
	req := strings.Join([]string{
		`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}`,
		`{"jsonrpc":"2.0","id":2,"method":"tools/list"}`,
		``,
	}, "\n")
	out := new(bytes.Buffer)
	code := Run(context.Background(), Invocation{
		Args:    []string{"mcp", "--repo", repo},
		Dir:     repo,
		Env:     OverlayEnv(OSEnv(), cliEnvFor(t)),
		Stdin:   strings.NewReader(req),
		Stdout:  out,
		Stderr:  new(bytes.Buffer),
		TempDir: t.TempDir(),
	})
	require.Equal(t, 0, code)
	var list struct {
		Result struct {
			Tools []struct {
				Name string `json:"name"`
			} `json:"tools"`
		} `json:"result"`
	}
	lines := nonEmptyMCPLines(out.String())
	require.NoError(t, json.Unmarshal([]byte(lines[len(lines)-1]), &list))
	require.Len(t, list.Result.Tools, 1)
	assert.Equal(t, "arm", list.Result.Tools[0].Name)
}
