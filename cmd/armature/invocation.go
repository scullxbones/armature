package main

import (
	"context"
	"io"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/scullxbones/armature/internal/adapters"
	"github.com/scullxbones/armature/internal/claim"
	"github.com/scullxbones/armature/internal/ops"
	"github.com/scullxbones/armature/internal/tui"
	"github.com/spf13/cobra"
)

// Env looks up invocation environment variables. Command code must not call
// os.Getenv; main constructs an OSEnv for the process and tests pass overlays.
type Env interface {
	Lookup(key string) (string, bool)
}

type osEnv struct{}

func (osEnv) Lookup(key string) (string, bool) {
	return os.LookupEnv(key)
}

func (osEnv) Environ() []string {
	return os.Environ()
}

// OSEnv reads the process environment.
func OSEnv() Env { return osEnv{} }

type overlayEnv struct {
	top  map[string]string
	base Env
}

func (o overlayEnv) Lookup(key string) (string, bool) {
	if o.top != nil {
		if v, ok := o.top[key]; ok {
			return v, true
		}
	}
	if o.base != nil {
		return o.base.Lookup(key)
	}
	return "", false
}

func (o overlayEnv) Environ() []string {
	base := os.Environ()
	if e, ok := o.base.(interface{ Environ() []string }); ok {
		base = e.Environ()
	}
	return mergeEnviron(base, o.top)
}

func mergeEnviron(base []string, top map[string]string) []string {
	if len(top) == 0 {
		return append([]string(nil), base...)
	}
	skip := make(map[string]bool, len(top))
	for k := range top {
		skip[k] = true
	}
	out := make([]string, 0, len(base)+len(top))
	for _, kv := range base {
		name, _, _ := splitEnvKV(kv)
		if skip[name] {
			continue
		}
		out = append(out, kv)
	}
	for k, v := range top {
		out = append(out, k+"="+v)
	}
	return out
}

func splitEnvKV(kv string) (name, value string, ok bool) {
	for i := 0; i < len(kv); i++ {
		if kv[i] == '=' {
			return kv[:i], kv[i+1:], true
		}
	}
	return kv, "", false
}

// OverlayEnv returns values from top when present, otherwise base.
func OverlayEnv(base Env, top map[string]string) Env {
	if len(top) == 0 {
		if base == nil {
			return OSEnv()
		}
		return base
	}
	return overlayEnv{top: top, base: base}
}

// Invocation is the full input of one arm process. Tests and main() both call
// Run with one of these so format, env, cwd, and temp files are not process-global.
type Invocation struct {
	Args    []string
	Dir     string
	Env     Env
	Stdout  io.Writer
	Stderr  io.Writer
	TTY     bool
	TempDir string
	// replayLeases overrides claim.Owners for this invocation only (tests).
	replayLeases          func([]ops.Op) map[string]claim.Lease
	afterIdempotencyCheck func()
}

type session struct {
	inv Invocation
	ui  *tui.Renderer
}

type sessionKey struct{}

func envLookup(env Env, key string) string {
	if env == nil {
		return ""
	}
	v, ok := env.Lookup(key)
	if !ok {
		return ""
	}
	return v
}

func newSession(inv Invocation) *session {
	inv = normalizeInvocation(inv)
	return &session{
		inv: inv,
		ui:  tui.NewRenderer(inv.TTY, inv.Env.Lookup),
	}
}

func normalizeInvocation(inv Invocation) Invocation {
	if inv.Env == nil {
		inv.Env = OSEnv()
	}
	if inv.Stdout == nil {
		inv.Stdout = os.Stdout
	}
	if inv.Stderr == nil {
		inv.Stderr = os.Stderr
	}
	if inv.TempDir == "" {
		inv.TempDir = os.TempDir()
	}
	return inv
}

// OSInvocation is the process-shaped Invocation used by main().
func OSInvocation(args []string, stdout, stderr io.Writer) Invocation {
	return Invocation{
		Args:    args,
		Env:     OSEnv(),
		Stdout:  stdout,
		Stderr:  stderr,
		TTY:     tui.StdoutIsTerminal(),
		TempDir: os.TempDir(),
	}
}

func attachSession(cmd *cobra.Command, sess *session) {
	if cmd == nil || sess == nil {
		return
	}
	base := cmd.Context()
	if base == nil {
		base = context.Background()
	}
	cmd.SetContext(context.WithValue(base, sessionKey{}, sess))
}

func sessionFrom(cmd *cobra.Command) *session {
	for c := cmd; c != nil; c = c.Parent() {
		ctx := c.Context()
		if ctx == nil {
			continue
		}
		if sess, ok := ctx.Value(sessionKey{}).(*session); ok && sess != nil {
			return sess
		}
		if c.Parent() == nil {
			break
		}
	}
	return nil
}

func uiFrom(cmd *cobra.Command) *tui.Renderer {
	if sess := sessionFrom(cmd); sess != nil {
		return sess.ui
	}
	return tui.NewRenderer(false, OSEnv().Lookup)
}

func envFrom(cmd *cobra.Command) Env {
	if sess := sessionFrom(cmd); sess != nil && sess.inv.Env != nil {
		return sess.inv.Env
	}
	return OSEnv()
}

func tempDirFrom(cmd *cobra.Command) string {
	if sess := sessionFrom(cmd); sess != nil && sess.inv.TempDir != "" {
		return sess.inv.TempDir
	}
	return os.TempDir()
}

func invocationDir(cmd *cobra.Command) string {
	if sess := sessionFrom(cmd); sess != nil && sess.inv.Dir != "" {
		return sess.inv.Dir
	}
	return ""
}

func replayLeasesFrom(cmd *cobra.Command) func([]ops.Op) map[string]claim.Lease {
	if sess := sessionFrom(cmd); sess != nil && sess.inv.replayLeases != nil {
		return sess.inv.replayLeases
	}
	return claim.Owners
}

func environFrom(cmd *cobra.Command) []string {
	env := envFrom(cmd)
	if e, ok := env.(interface{ Environ() []string }); ok {
		return e.Environ()
	}
	return os.Environ()
}

func envSliceVal(environ []string, key string) string {
	prefix := key + "="
	for i := len(environ) - 1; i >= 0; i-- {
		if len(environ[i]) >= len(prefix) && environ[i][:len(prefix)] == prefix {
			return environ[i][len(prefix):]
		}
	}
	return ""
}

func lookPathOnEnv(pathList, file string) (string, error) {
	for _, dir := range filepath.SplitList(pathList) {
		if dir == "" {
			continue
		}
		candidate := filepath.Join(dir, file)
		st, err := os.Stat(candidate)
		if err != nil || st.IsDir() {
			continue
		}
		if st.Mode()&0o111 == 0 {
			continue
		}
		return candidate, nil
	}
	return "", os.ErrNotExist
}

func gitCommand(environ []string, args ...string) *exec.Cmd {
	gitBin := "git"
	if path := envSliceVal(environ, "PATH"); path != "" {
		if abs, err := lookPathOnEnv(path, "git"); err == nil {
			gitBin = abs
		}
	}
	cmd := exec.CommandContext(context.Background(), gitBin, args...) //nolint:gosec // G204: git binary plus caller-built args
	if len(environ) > 0 {
		cmd.Env = append([]string(nil), environ...)
	}
	return cmd
}

func gitWith(env Env, repoPath string) *adapters.Client {
	c := adapters.New(repoPath)
	if e, ok := env.(interface{ Environ() []string }); ok {
		return c.WithEnviron(e.Environ())
	}
	return c
}

func gitAt(cmd *cobra.Command, repoPath string) *adapters.Client {
	return gitWith(envFrom(cmd), repoPath)
}

func gitIsolatedAt(cmd *cobra.Command, repoPath string) *adapters.Client {
	return adapters.NewIsolated(repoPath).WithEnviron(environFrom(cmd))
}

func repoPathFromCmd(cmd *cobra.Command) string {
	if cmd == nil || cmd.Root() == nil {
		if dir := invocationDir(cmd); dir != "" {
			return dir
		}
		return "."
	}
	path, err := cmd.Root().PersistentFlags().GetString("repo")
	if err != nil || path == "" {
		if dir := invocationDir(cmd); dir != "" {
			return dir
		}
		return "."
	}
	if !filepath.IsAbs(path) {
		if dir := invocationDir(cmd); dir != "" {
			return filepath.Join(dir, path)
		}
	}
	return path
}

func commandFor(inv Invocation) *cobra.Command {
	inv = normalizeInvocation(inv)
	sess := newSession(inv)
	root := newRootCmd()
	attachSession(root, sess)
	root.SetOut(inv.Stdout)
	root.SetErr(inv.Stderr)
	return root
}

// Run is the only CLI entrypoint. main() and tests both call it.
//
//nolint:contextcheck // cobra command tree and git helpers take context from cmd.Context after SetContext
func Run(ctx context.Context, inv Invocation) int {
	root := commandFor(inv)
	if sess := sessionFrom(root); sess != nil {
		root.SetContext(context.WithValue(ctx, sessionKey{}, sess))
	} else {
		root.SetContext(ctx)
	}
	return executeRoot(root, inv.Args, inv.Stdout, inv.Stderr)
}
