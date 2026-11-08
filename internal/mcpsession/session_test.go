package mcpsession

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestServeToolsListIsOneArmTool_REQ_MCPSTDIO_S1_T3(t *testing.T) {
	t.Parallel()

	in := strings.NewReader(strings.Join([]string{
		`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}`,
		`{"jsonrpc":"2.0","method":"notifications/initialized"}`,
		`{"jsonrpc":"2.0","id":2,"method":"tools/list","params":{}}`,
		``,
	}, "\n"))
	var out bytes.Buffer
	exit := Serve(context.Background(), in, &out, nil, "test-version", nil)
	require.Equal(t, ExitClientClose, exit)

	lines := nonEmptyLines(out.String())
	require.GreaterOrEqual(t, len(lines), 2)

	var listResp struct {
		Result struct {
			Tools []struct {
				Name        string         `json:"name"`
				InputSchema map[string]any `json:"inputSchema"`
			} `json:"tools"`
		} `json:"result"`
	}
	require.NoError(t, json.Unmarshal([]byte(lines[len(lines)-1]), &listResp))
	require.Len(t, listResp.Result.Tools, 1)
	assert.Equal(t, "arm", listResp.Result.Tools[0].Name)
	props, ok := listResp.Result.Tools[0].InputSchema["properties"].(map[string]any)
	require.True(t, ok)
	_, hasArgs := props["args"]
	assert.True(t, hasArgs, "arm tool must expose args")
}

func TestServeRunsOneCallAtATime_REQ_MCPSTDIO_S1_T3(t *testing.T) {
	t.Parallel()

	var concurrent int32
	var maxConcurrent int32
	var mu sync.Mutex
	var order []string

	run := func(ctx context.Context, c Call) Result {
		n := atomic.AddInt32(&concurrent, 1)
		for {
			old := atomic.LoadInt32(&maxConcurrent)
			if n <= old || atomic.CompareAndSwapInt32(&maxConcurrent, old, n) {
				break
			}
		}
		defer atomic.AddInt32(&concurrent, -1)
		time.Sleep(30 * time.Millisecond)
		mu.Lock()
		order = append(order, strings.Join(c.Args, " "))
		mu.Unlock()
		return Result{Stdout: []byte("ok:" + strings.Join(c.Args, " ")), ExitCode: 0}
	}

	req1 := `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"arm","arguments":{"args":["one"]}}}`
	req2 := `{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"arm","arguments":{"args":["two"]}}}`
	in := strings.NewReader(req1 + "\n" + req2 + "\n")
	var out bytes.Buffer
	exit := Serve(context.Background(), in, &out, nil, "v", run)
	require.Equal(t, ExitClientClose, exit)
	assert.Equal(t, int32(1), atomic.LoadInt32(&maxConcurrent), "Runner must never run concurrently")
	mu.Lock()
	assert.Equal(t, []string{"one", "two"}, order)
	mu.Unlock()
}

func TestMCPSessionBansNetExecAndTicker_REQ_MCPSTDIO_S1_T3(t *testing.T) {
	t.Parallel()

	_, thisFile, _, ok := runtime.Caller(0)
	require.True(t, ok)
	pkgDir := filepath.Dir(thisFile)

	entries, err := os.ReadDir(pkgDir)
	require.NoError(t, err)

	bannedImports := map[string]bool{
		`"net"`:      true,
		`"os/exec"`:  true,
		`"net/http"`: true,
	}
	fset := token.NewFileSet()
	var violations []string
	for _, entry := range entries {
		name := entry.Name()
		if !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		path := filepath.Join(pkgDir, name)
		file, parseErr := parser.ParseFile(fset, path, nil, 0)
		require.NoError(t, parseErr, name)
		for _, spec := range file.Imports {
			if bannedImports[spec.Path.Value] {
				violations = append(violations, fmt.Sprintf("%s imports %s", name, spec.Path.Value))
			}
		}
		ast.Inspect(file, func(n ast.Node) bool {
			sel, ok := n.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			pkgIdent, ok := sel.X.(*ast.Ident)
			if !ok {
				return true
			}
			if pkgIdent.Name == "time" && sel.Sel.Name == "NewTicker" {
				pos := fset.Position(n.Pos())
				violations = append(violations, fmt.Sprintf("%s:%d calls time.NewTicker", name, pos.Line))
			}
			return true
		})
	}

	sessionFile := filepath.Join(pkgDir, "session.go")
	src, err := os.ReadFile(sessionFile)
	require.NoError(t, err)
	file, err := parser.ParseFile(fset, sessionFile, src, 0)
	require.NoError(t, err)
	for _, decl := range file.Decls {
		gen, ok := decl.(*ast.GenDecl)
		if !ok {
			continue
		}
		for _, spec := range gen.Specs {
			ts, ok := spec.(*ast.TypeSpec)
			if !ok || (ts.Name.Name != "session" && ts.Name.Name != "Session") {
				continue
			}
			st, ok := ts.Type.(*ast.StructType)
			if !ok {
				continue
			}
			for _, field := range st.Fields.List {
				for _, name := range field.Names {
					lower := strings.ToLower(name.Name)
					if strings.Contains(lower, "worker") ||
						strings.Contains(lower, "listen") ||
						strings.Contains(lower, "addr") ||
						strings.Contains(lower, "timer") ||
						strings.Contains(lower, "ticker") ||
						strings.Contains(lower, "context") {
						violations = append(violations, fmt.Sprintf("session type field %s forbidden", name.Name))
					}
				}
			}
		}
	}
	require.Empty(t, violations, strings.Join(violations, "\n"))
}

func TestServeExitsOnEOF(t *testing.T) {
	t.Parallel()
	exit := Serve(context.Background(), strings.NewReader(""), io.Discard, nil, "v", nil)
	assert.Equal(t, ExitClientClose, exit)
}

func TestServeToolsCallReturnsStdout(t *testing.T) {
	t.Parallel()
	run := func(_ context.Context, c Call) Result {
		return Result{Stdout: []byte("hello " + strings.Join(c.Args, ",")), ExitCode: 0}
	}
	in := strings.NewReader(`{"jsonrpc":"2.0","id":9,"method":"tools/call","params":{"name":"arm","arguments":{"args":["ready"]}}}` + "\n")
	var out bytes.Buffer
	exit := Serve(context.Background(), in, &out, nil, "v", run)
	require.Equal(t, ExitClientClose, exit)
	assert.Contains(t, out.String(), `"hello ready"`)
	assert.Contains(t, out.String(), `"exitCode":0`)
}

func TestServeBrokenWire(t *testing.T) {
	t.Parallel()
	exit := Serve(context.Background(), strings.NewReader("not-json\n"), io.Discard, nil, "v", nil)
	assert.Equal(t, ExitWireBroken, exit)
}

func TestServeRunnerPanic(t *testing.T) {
	t.Parallel()
	run := func(context.Context, Call) Result {
		panic("boom")
	}
	in := strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"arm","arguments":{"args":["x"]}}}` + "\n")
	exit := Serve(context.Background(), in, io.Discard, nil, "v", run)
	assert.Equal(t, ExitRunnerPanic, exit)
}

func TestServeEOFDuringBlockingCallCancelsRunner_REQ_MCPSTDIO_S1(t *testing.T) {
	t.Parallel()

	pr, pw := io.Pipe()
	started := make(chan struct{})
	cancelled := make(chan struct{})
	run := func(ctx context.Context, _ Call) Result {
		close(started)
		select {
		case <-ctx.Done():
			close(cancelled)
			return Result{ExitCode: 1, Stdout: []byte("cancelled\n")}
		case <-time.After(5 * time.Second):
			t.Error("runner was not cancelled after stdin EOF")
			return Result{ExitCode: 1}
		}
	}

	done := make(chan Exit, 1)
	go func() {
		done <- Serve(context.Background(), pr, io.Discard, nil, "v", run)
	}()

	_, err := io.WriteString(pw, `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"arm","arguments":{"args":["slow"]}}}`+"\n")
	require.NoError(t, err)
	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("runner never started")
	}
	require.NoError(t, pw.Close()) // stdin EOF while call is in flight

	select {
	case <-cancelled:
	case <-time.After(2 * time.Second):
		t.Fatal("runner ctx was not cancelled on stdin EOF")
	}
	select {
	case exit := <-done:
		assert.Equal(t, ExitClientClose, exit)
	case <-time.After(2 * time.Second):
		t.Fatal("Serve did not return after stdin EOF during tools/call")
	}
}

func TestServeContextCancelDuringBlockingCall_REQ_MCPSTDIO_S1(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	started := make(chan struct{})
	cancelled := make(chan struct{})
	run := func(callCtx context.Context, _ Call) Result {
		close(started)
		select {
		case <-callCtx.Done():
			close(cancelled)
			return Result{ExitCode: 1}
		case <-time.After(5 * time.Second):
			t.Error("runner was not cancelled after parent context cancel")
			return Result{ExitCode: 1}
		}
	}

	// Keep stdin open so EOF is not the stop signal; only context cancel is.
	pr, pw := io.Pipe()
	t.Cleanup(func() { require.NoError(t, pw.Close()) })

	done := make(chan Exit, 1)
	go func() {
		done <- Serve(ctx, pr, io.Discard, nil, "v", run)
	}()

	_, err := io.WriteString(pw, `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"arm","arguments":{"args":["slow"]}}}`+"\n")
	require.NoError(t, err)
	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("runner never started")
	}
	cancel()

	select {
	case <-cancelled:
	case <-time.After(2 * time.Second):
		t.Fatal("runner ctx was not cancelled on parent context cancel")
	}
	select {
	case exit := <-done:
		assert.Equal(t, ExitClientClose, exit)
	case <-time.After(2 * time.Second):
		t.Fatal("Serve did not return after context cancel during tools/call")
	}
}

func TestServeSignalGraceAndSecondSignal_REQ_MCPSTDIO_S1(t *testing.T) {
	t.Parallel()

	sig := make(chan os.Signal, 2)
	pr, pw := io.Pipe()
	started := make(chan struct{})
	run := func(ctx context.Context, _ Call) Result {
		close(started)
		<-ctx.Done()
		return Result{ExitCode: 1}
	}
	done := make(chan Exit, 1)
	go func() {
		done <- Serve(context.Background(), pr, io.Discard, sig, "v", run)
	}()
	_, err := io.WriteString(pw, `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"arm","arguments":{"args":["slow"]}}}`+"\n")
	require.NoError(t, err)
	<-started

	sig <- os.Interrupt
	sig <- os.Interrupt // second signal during grace
	select {
	case exit := <-done:
		assert.Equal(t, ExitSignaledAgain, exit)
	case <-time.After(2 * time.Second):
		t.Fatal("Serve did not return ExitSignaledAgain")
	}
	require.NoError(t, pw.Close())
}

func TestServeSignalGraceWithoutSecondSignal_REQ_MCPSTDIO_S1(t *testing.T) {
	t.Parallel()

	sig := make(chan os.Signal, 1)
	started := make(chan struct{})
	run := func(ctx context.Context, _ Call) Result {
		close(started)
		<-ctx.Done()
		return Result{ExitCode: 1}
	}
	pr, pw := io.Pipe()
	done := make(chan Exit, 1)
	go func() {
		done <- Serve(context.Background(), pr, io.Discard, sig, "v", run)
	}()
	_, err := io.WriteString(pw, `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"arm","arguments":{"args":["slow"]}}}`+"\n")
	require.NoError(t, err)
	<-started
	sig <- os.Interrupt
	select {
	case exit := <-done:
		assert.Equal(t, ExitSignaled, exit)
	case <-time.After(2 * time.Second):
		t.Fatal("Serve did not return ExitSignaled after grace")
	}
	require.NoError(t, pw.Close())
}

func TestServeWriterFailureIsWireBroken_REQ_MCPSTDIO_S1(t *testing.T) {
	t.Parallel()
	in := strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"tools/list","params":{}}` + "\n")
	exit := Serve(context.Background(), in, errWriter{}, nil, "v", nil)
	assert.Equal(t, ExitWireBroken, exit)
}

func TestServeReaderFailureIsWireBroken_REQ_MCPSTDIO_S1(t *testing.T) {
	t.Parallel()
	exit := Serve(context.Background(), errReader{}, io.Discard, nil, "v", nil)
	assert.Equal(t, ExitWireBroken, exit)
}

func TestServeMalformedParamsReturnsRPCErrorAndContinues_REQ_MCPSTDIO_S1(t *testing.T) {
	t.Parallel()
	in := strings.NewReader(strings.Join([]string{
		`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":"not-an-object"}`,
		`{"jsonrpc":"2.0","id":2,"method":"tools/list","params":{}}`,
		``,
	}, "\n"))
	var out bytes.Buffer
	exit := Serve(context.Background(), in, &out, nil, "v", nil)
	require.Equal(t, ExitClientClose, exit)
	assert.Contains(t, out.String(), `"invalid params"`)
	assert.Contains(t, out.String(), `"name":"arm"`)
}

func TestServeMalformedArgumentsReturnsRPCErrorAndContinues_REQ_MCPSTDIO_S1(t *testing.T) {
	t.Parallel()
	in := strings.NewReader(strings.Join([]string{
		`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"arm","arguments":"bad"}}`,
		`{"jsonrpc":"2.0","id":2,"method":"ping","params":{}}`,
		``,
	}, "\n"))
	var out bytes.Buffer
	exit := Serve(context.Background(), in, &out, nil, "v", nil)
	require.Equal(t, ExitClientClose, exit)
	assert.Contains(t, out.String(), `"invalid arguments"`)
	assert.Contains(t, out.String(), `"id":2`)
}

func TestToolResultTextSurfacesFoldedStdout(t *testing.T) {
	t.Parallel()
	assert.Equal(t, "out", toolResultText(Result{Stdout: []byte("out"), ExitCode: 1}))
	assert.Equal(t, "err\n", toolResultText(Result{Stdout: []byte("err\n"), ExitCode: 2}))
	assert.Equal(t, "", toolResultText(Result{ExitCode: 0}))
}

type errWriter struct{}

func (errWriter) Write([]byte) (int, error) { return 0, fmt.Errorf("write failed") }

type errReader struct{}

func (errReader) Read([]byte) (int, error) { return 0, fmt.Errorf("read failed") }

func nonEmptyLines(s string) []string {
	var out []string
	for _, line := range strings.Split(s, "\n") {
		if strings.TrimSpace(line) != "" {
			out = append(out, line)
		}
	}
	return out
}
