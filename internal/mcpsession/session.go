// Package mcpsession serves one newline-delimited MCP session over stdio.
// It imports neither net nor os/exec. Lifetime is the caller's pipes (ADR 0024).
package mcpsession

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"time"
)

// Call is one tools/call of the single tool named arm.
type Call struct {
	Args []string
}

// Result is the stdout and exit code of one in-process arm command.
type Result struct {
	Stdout   []byte
	ExitCode int
}

// Runner runs one Call. Serve never calls Runner concurrently.
type Runner func(ctx context.Context, c Call) Result

// Exit is why Serve returned.
type Exit int

const (
	// ExitClientClose means the client closed stdin (EOF).
	ExitClientClose Exit = iota
	// ExitSignaled means the session received a stop signal and drained.
	ExitSignaled
	// ExitSignaledAgain means a second signal arrived during the grace wait.
	ExitSignaledAgain
	// ExitRunnerPanic means Runner panicked.
	ExitRunnerPanic
	// ExitWireBroken means the request line was not valid JSON-RPC.
	ExitWireBroken
)

const signalGrace = 500 * time.Millisecond

const toolName = "arm"

type jsonRPCRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params"`
}

type jsonRPCResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Result  any             `json:"result,omitempty"`
	Error   *rpcError       `json:"error,omitempty"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type toolsCallParams struct {
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments"`
}

type armArguments struct {
	Args []string `json:"args"`
}

// Serve reads one JSON object per line from in, writes one response per
// request to out, and returns when the client closes, signals, the wire
// breaks, or Runner panics. version is reported in initialize.
func Serve(ctx context.Context, in io.Reader, out io.Writer, sig <-chan os.Signal, version string, run Runner) Exit {
	if run == nil {
		run = func(context.Context, Call) Result { return Result{} }
	}
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	lines := make(chan lineEvent, 1)
	go readLines(in, lines)

	var writeMu sync.Mutex
	write := func(v any) error {
		writeMu.Lock()
		defer writeMu.Unlock()
		b, err := json.Marshal(v)
		if err != nil {
			return err
		}
		_, err = out.Write(append(b, '\n'))
		return err
	}

	for {
		select {
		case <-ctx.Done():
			return ExitClientClose
		case s := <-sig:
			_ = s
			cancel()
			return waitSignalExit(sig)
		case ev, ok := <-lines:
			if !ok || ev.err == io.EOF {
				return ExitClientClose
			}
			if ev.err != nil {
				return ExitWireBroken
			}
			req, err := decodeRequest(ev.line)
			if err != nil {
				return ExitWireBroken
			}
			if req.Method == "" {
				return ExitWireBroken
			}
			if isNotification(req) {
				continue
			}
			exit, handled := handleRequest(ctx, req, version, run, write, sig, cancel)
			if handled {
				if exit != -1 {
					return exit
				}
				continue
			}
		}
	}
}

type lineEvent struct {
	line string
	err  error
}

func readLines(in io.Reader, out chan<- lineEvent) {
	defer close(out)
	sc := bufio.NewScanner(in)
	// MCP initialize payloads are small; raise the limit for long tool results on the request side.
	sc.Buffer(make([]byte, 0, 64*1024), 8*1024*1024)
	for sc.Scan() {
		out <- lineEvent{line: sc.Text()}
	}
	if err := sc.Err(); err != nil {
		out <- lineEvent{err: err}
		return
	}
	out <- lineEvent{err: io.EOF}
}

func decodeRequest(line string) (jsonRPCRequest, error) {
	line = strings.TrimSpace(line)
	if line == "" {
		return jsonRPCRequest{}, fmt.Errorf("empty line")
	}
	var req jsonRPCRequest
	if err := json.Unmarshal([]byte(line), &req); err != nil {
		return jsonRPCRequest{}, err
	}
	if req.JSONRPC != "" && req.JSONRPC != "2.0" {
		return jsonRPCRequest{}, fmt.Errorf("unsupported jsonrpc %q", req.JSONRPC)
	}
	return req, nil
}

func isNotification(req jsonRPCRequest) bool {
	return len(req.ID) == 0 || string(req.ID) == "null"
}

func handleRequest(
	ctx context.Context,
	req jsonRPCRequest,
	version string,
	run Runner,
	write func(any) error,
	sig <-chan os.Signal,
	cancel context.CancelFunc,
) (Exit, bool) {
	switch req.Method {
	case "initialize":
		result := map[string]any{
			"protocolVersion": "2024-11-05",
			"capabilities": map[string]any{
				"tools": map[string]any{},
			},
			"serverInfo": map[string]any{
				"name":    "armature",
				"version": version,
			},
		}
		if err := write(jsonRPCResponse{JSONRPC: "2.0", ID: req.ID, Result: result}); err != nil {
			return ExitWireBroken, true
		}
		return -1, true
	case "ping":
		if err := write(jsonRPCResponse{JSONRPC: "2.0", ID: req.ID, Result: map[string]any{}}); err != nil {
			return ExitWireBroken, true
		}
		return -1, true
	case "tools/list":
		result := map[string]any{
			"tools": []map[string]any{
				{
					"name":        toolName,
					"description": "Run one arm CLI command inside this process. Args is the argv after arm.",
					"inputSchema": map[string]any{
						"type": "object",
						"properties": map[string]any{
							"args": map[string]any{
								"type":  "array",
								"items": map[string]any{"type": "string"},
							},
						},
						"required": []string{"args"},
					},
				},
			},
		}
		if err := write(jsonRPCResponse{JSONRPC: "2.0", ID: req.ID, Result: result}); err != nil {
			return ExitWireBroken, true
		}
		return -1, true
	case "tools/call":
		return handleToolsCall(ctx, req, run, write, sig, cancel)
	default:
		err := write(jsonRPCResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Error:   &rpcError{Code: -32601, Message: "method not found: " + req.Method},
		})
		if err != nil {
			return ExitWireBroken, true
		}
		return -1, true
	}
}

func handleToolsCall(
	ctx context.Context,
	req jsonRPCRequest,
	run Runner,
	write func(any) error,
	sig <-chan os.Signal,
	cancel context.CancelFunc,
) (Exit, bool) {
	var params toolsCallParams
	if err := json.Unmarshal(req.Params, &params); err != nil {
		if werr := write(jsonRPCResponse{JSONRPC: "2.0", ID: req.ID, Error: &rpcError{Code: -32602, Message: "invalid params"}}); werr != nil {
			return ExitWireBroken, true
		}
		return ExitWireBroken, true
	}
	if params.Name != toolName {
		if werr := write(jsonRPCResponse{JSONRPC: "2.0", ID: req.ID, Error: &rpcError{Code: -32602, Message: "unknown tool: " + params.Name}}); werr != nil {
			return ExitWireBroken, true
		}
		return -1, true
	}
	var args armArguments
	if len(params.Arguments) > 0 {
		if err := json.Unmarshal(params.Arguments, &args); err != nil {
			if werr := write(jsonRPCResponse{JSONRPC: "2.0", ID: req.ID, Error: &rpcError{Code: -32602, Message: "invalid arguments"}}); werr != nil {
				return ExitWireBroken, true
			}
			return ExitWireBroken, true
		}
	}

	type callOutcome struct {
		res Result
		pan any
	}
	done := make(chan callOutcome, 1)
	go func() {
		defer func() {
			if r := recover(); r != nil {
				done <- callOutcome{pan: r}
			}
		}()
		done <- callOutcome{res: run(ctx, Call(args))}
	}()

	select {
	case <-sig:
		cancel()
		return waitSignalExit(sig), true
	case out := <-done:
		if out.pan != nil {
			return ExitRunnerPanic, true
		}
		text := string(out.res.Stdout)
		result := map[string]any{
			"content": []map[string]any{
				{"type": "text", "text": text},
			},
			"structuredContent": map[string]any{
				"exitCode": out.res.ExitCode,
				"stdout":   text,
			},
			"isError": out.res.ExitCode != 0,
		}
		if err := write(jsonRPCResponse{JSONRPC: "2.0", ID: req.ID, Result: result}); err != nil {
			return ExitWireBroken, true
		}
		return -1, true
	}
}

func waitSignalExit(sig <-chan os.Signal) Exit {
	timer := time.NewTimer(signalGrace)
	defer timer.Stop()
	select {
	case <-sig:
		return ExitSignaledAgain
	case <-timer.C:
		return ExitSignaled
	}
}
