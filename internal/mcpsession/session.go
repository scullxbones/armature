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

// Result is the captured output and exit code of one in-process arm command.
// Stderr-only failures must fold diagnostics into Stdout at the Runner
// boundary (C10: no exported Result.Stderr). toolResultText and
// structuredContent then surface those bytes without a wider package API.
type Result struct {
	Stdout   []byte
	ExitCode int
}

// Runner runs one Call. Serve never calls Runner concurrently.
// Runner must honor ctx cancellation (ADR 0024: session exits on client close).
type Runner func(ctx context.Context, c Call) Result

// Exit is why Serve returned.
type Exit int

const (
	// ExitClientClose means the client closed stdin (EOF) or the parent context ended.
	ExitClientClose Exit = iota
	// ExitSignaled means the session received a stop signal and drained.
	ExitSignaled
	// ExitSignaledAgain means a second signal arrived during the grace wait.
	ExitSignaledAgain
	// ExitRunnerPanic means Runner panicked.
	ExitRunnerPanic
	// ExitWireBroken means the request line was not valid JSON-RPC or the writer failed.
	ExitWireBroken
)

// continueSession is the sentinel Exit from handleRequest meaning "keep serving".
const continueSession Exit = -1

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

type lineEvent struct {
	line string
	err  error
}

// lineInbox merges a pending queue (lines arrived during an in-flight call)
// with the live reader channel so Serve stays serial while still seeing EOF.
type lineInbox struct {
	live    <-chan lineEvent
	pending []lineEvent
}

func (q *lineInbox) push(ev lineEvent) {
	q.pending = append(q.pending, ev)
}

func (q *lineInbox) popPending() (lineEvent, bool) {
	if len(q.pending) == 0 {
		return lineEvent{}, false
	}
	ev := q.pending[0]
	q.pending = q.pending[1:]
	return ev, true
}

// Serve reads one JSON object per line from in, writes one response per
// request to out, and returns when the client closes, signals, the wire
// breaks, or Runner panics. version is reported in initialize.
//
// An in-flight tools/call is cancelled when stdin hits EOF, the parent
// context ends, or a stop signal arrives (ADR 0024) — independent of any
// requests already queued. Extra request lines that arrive during a call
// are queued and handled after the call returns only if the session continues.
func Serve(ctx context.Context, in io.Reader, out io.Writer, sig <-chan os.Signal, version string, run Runner) Exit {
	if run == nil {
		run = func(context.Context, Call) Result { return Result{} }
	}
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	live := make(chan lineEvent, 1)
	go readLines(ctx, in, live)
	inbox := &lineInbox{live: live}

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
		if ev, ok := inbox.popPending(); ok {
			if exit := dispatchLine(ctx, ev, true, version, run, write, sig, cancel, inbox); exit != continueSession {
				return exit
			}
			continue
		}
		select {
		case <-ctx.Done():
			return ExitClientClose
		case s := <-sig:
			_ = s
			cancel()
			return waitSignalExit(sig)
		case ev, ok := <-inbox.live:
			if !ok {
				cancel()
				return ExitClientClose
			}
			if exit := dispatchLine(ctx, ev, true, version, run, write, sig, cancel, inbox); exit != continueSession {
				return exit
			}
		}
	}
}

func dispatchLine(
	ctx context.Context,
	ev lineEvent,
	ok bool,
	version string,
	run Runner,
	write func(any) error,
	sig <-chan os.Signal,
	cancel context.CancelFunc,
	inbox *lineInbox,
) Exit {
	if exit, done := classifyLineEvent(ev, ok); done {
		cancel()
		return exit
	}
	req, err := decodeRequest(ev.line)
	if err != nil || req.Method == "" {
		cancel()
		return ExitWireBroken
	}
	if isNotification(req) {
		return continueSession
	}
	return handleRequest(ctx, req, version, run, write, sig, cancel, inbox)
}

func classifyLineEvent(ev lineEvent, ok bool) (Exit, bool) {
	if !ok || ev.err == io.EOF {
		return ExitClientClose, true
	}
	if ev.err != nil {
		return ExitWireBroken, true
	}
	return continueSession, false
}

// readLines scans in and sends one event per line. Sends are cancelled when
// ctx ends so the goroutine cannot block forever on a full channel after Serve
// returns. A blocked Scan still waits on the reader (EOF unblocks it).
func readLines(ctx context.Context, in io.Reader, out chan<- lineEvent) {
	defer close(out)
	sc := bufio.NewScanner(in)
	sc.Buffer(make([]byte, 0, 64*1024), 8*1024*1024)
	for sc.Scan() {
		select {
		case <-ctx.Done():
			return
		case out <- lineEvent{line: sc.Text()}:
		}
	}
	ev := lineEvent{err: io.EOF}
	if err := sc.Err(); err != nil {
		ev.err = err
	}
	select {
	case <-ctx.Done():
		return
	case out <- ev:
	}
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
	inbox *lineInbox,
) Exit {
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
			cancel()
			return ExitWireBroken
		}
		return continueSession
	case "ping":
		if err := write(jsonRPCResponse{JSONRPC: "2.0", ID: req.ID, Result: map[string]any{}}); err != nil {
			cancel()
			return ExitWireBroken
		}
		return continueSession
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
			cancel()
			return ExitWireBroken
		}
		return continueSession
	case "tools/call":
		return handleToolsCall(ctx, req, run, write, sig, cancel, inbox)
	default:
		if err := write(jsonRPCResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Error:   &rpcError{Code: -32601, Message: "method not found: " + req.Method},
		}); err != nil {
			cancel()
			return ExitWireBroken
		}
		return continueSession
	}
}

func handleToolsCall(
	ctx context.Context,
	req jsonRPCRequest,
	run Runner,
	write func(any) error,
	sig <-chan os.Signal,
	cancel context.CancelFunc,
	inbox *lineInbox,
) Exit {
	var params toolsCallParams
	if len(req.Params) == 0 {
		if err := write(jsonRPCResponse{JSONRPC: "2.0", ID: req.ID, Error: &rpcError{Code: -32602, Message: "invalid params"}}); err != nil {
			cancel()
			return ExitWireBroken
		}
		return continueSession
	}
	if err := json.Unmarshal(req.Params, &params); err != nil {
		if werr := write(jsonRPCResponse{JSONRPC: "2.0", ID: req.ID, Error: &rpcError{Code: -32602, Message: "invalid params"}}); werr != nil {
			cancel()
			return ExitWireBroken
		}
		return continueSession
	}
	if params.Name != toolName {
		if werr := write(jsonRPCResponse{JSONRPC: "2.0", ID: req.ID, Error: &rpcError{Code: -32602, Message: "unknown tool: " + params.Name}}); werr != nil {
			cancel()
			return ExitWireBroken
		}
		return continueSession
	}
	var args armArguments
	if len(params.Arguments) > 0 {
		if err := json.Unmarshal(params.Arguments, &args); err != nil {
			if werr := write(jsonRPCResponse{JSONRPC: "2.0", ID: req.ID, Error: &rpcError{Code: -32602, Message: "invalid arguments"}}); werr != nil {
				cancel()
				return ExitWireBroken
			}
			return continueSession
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

	finish := func(out callOutcome) Exit {
		if out.pan != nil {
			cancel()
			return ExitRunnerPanic
		}
		text := toolResultText(out.res)
		// stderr key stays for schema stability; diagnostics from stderr-only
		// failures are folded into Stdout by the Runner (see toolRunner).
		result := map[string]any{
			"content": []map[string]any{
				{"type": "text", "text": text},
			},
			"structuredContent": map[string]any{
				"exitCode": out.res.ExitCode,
				"stdout":   string(out.res.Stdout),
				"stderr":   "",
			},
			"isError": out.res.ExitCode != 0,
		}
		if err := write(jsonRPCResponse{JSONRPC: "2.0", ID: req.ID, Result: result}); err != nil {
			cancel()
			return ExitWireBroken
		}
		return continueSession
	}

	// stopForClientClose handles stdin EOF / parent cancel during a call.
	// If the Runner already finished (eager EOF after a finite Reader's last
	// request), write that result first. Otherwise cancel and wait briefly.
	stopForClientClose := func() Exit {
		select {
		case out := <-done:
			if exit := finish(out); exit != continueSession {
				return exit
			}
			cancel()
			return ExitClientClose
		default:
		}
		cancel()
		timer := time.NewTimer(signalGrace)
		defer timer.Stop()
		select {
		case out := <-done:
			if exit := finish(out); exit != continueSession {
				return exit
			}
			return ExitClientClose
		case <-timer.C:
			return ExitClientClose
		}
	}

	for {
		select {
		case <-ctx.Done():
			return stopForClientClose()
		case <-sig:
			cancel()
			return waitSignalExit(sig)
		case ev, ok := <-inbox.live:
			if !ok {
				// Closed live channel is EOF. Do not re-enter this select arm:
				// a closed channel stays readable and would spin, appending
				// synthetic EOFs to pending (CPU/memory growth).
				return stopForClientClose()
			}
			if exit, terminal := classifyLineEvent(ev, true); terminal {
				if exit != ExitClientClose {
					cancel()
					return exit
				}
				// Stdin EOF is always terminal for the active call (ADR 0024),
				// regardless of requests already queued. stopForClientClose
				// cancels the Runner; queued work is abandoned with the session.
				return stopForClientClose()
			}
			// Another request arrived while this call runs: queue it; stay serial.
			inbox.push(ev)
		case out := <-done:
			return finish(out)
		}
	}
}

// toolResultText returns the Runner's captured Stdout. Callers that capture a
// separate stderr stream must fold stderr-only failure diagnostics into
// Stdout before returning Result (C10 / Deep Module: no Result.Stderr).
func toolResultText(res Result) string {
	return string(res.Stdout)
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
