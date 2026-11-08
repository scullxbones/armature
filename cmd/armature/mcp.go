package main

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"

	"github.com/scullxbones/armature/internal/mcpsession"
	"github.com/scullxbones/armature/internal/output"
	"github.com/spf13/cobra"
)

// mcpChdirMu serializes os.Chdir for arm mcp --repo so parallel tests and
// concurrent sessions cannot race the process working directory.
var mcpChdirMu sync.Mutex //nolint:gochecknoglobals // process cwd is process-global

func newMCPCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:               "mcp",
		Short:             "Serve one MCP stdio session with a single arm tool",
		Hidden:            true,
		SilenceErrors:     true,
		Annotations:       output.MarkProtocolOutput(nil),
		PersistentPreRunE: func(*cobra.Command, []string) error { return nil },
		RunE:              runMCP,
	}
	return cmd
}

func isMCPCommand(cmd *cobra.Command) bool {
	for c := cmd; c != nil; c = c.Parent() {
		if c.Name() == "mcp" {
			return true
		}
	}
	return false
}

func runMCP(cmd *cobra.Command, _ []string) error {
	if flag := cmd.Root().PersistentFlags().Lookup("worker-id"); flag != nil && flag.Changed {
		return skipCommandFailure(fmt.Errorf("--worker-id is not allowed on arm mcp; set ARM_WORKER_ID in the host env instead"))
	}

	repo, _ := cmd.Root().PersistentFlags().GetString("repo")
	if repo != "" {
		mcpChdirMu.Lock()
		defer mcpChdirMu.Unlock()
		cwd, err := os.Getwd()
		if err != nil {
			return skipCommandFailure(fmt.Errorf("getcwd: %w", err))
		}
		if err := os.Chdir(repo); err != nil {
			return skipCommandFailure(fmt.Errorf("chdir --repo: %w", err))
		}
		defer func() {
			if restoreErr := os.Chdir(cwd); restoreErr != nil {
				fmt.Fprintf(cmd.ErrOrStderr(), "mcp: restore cwd: %v\n", restoreErr)
			}
		}()
	}

	sess := sessionFrom(cmd)
	var outer Invocation
	if sess != nil {
		outer = sess.inv
	} else {
		outer = OSInvocation(nil, cmd.OutOrStdout(), cmd.ErrOrStderr())
	}
	if repo != "" {
		outer.Dir = repo
	}

	ctx := cmd.Context()
	if ctx == nil {
		ctx = context.Background()
	}
	sigCh := make(chan os.Signal, 2)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM, syscall.SIGHUP)
	defer signal.Stop(sigCh)

	exit := mcpsession.Serve(ctx, cmd.InOrStdin(), cmd.OutOrStdout(), sigCh, Version, toolRunner(outer))
	switch exit {
	case mcpsession.ExitClientClose, mcpsession.ExitSignaled, mcpsession.ExitSignaledAgain:
		return nil
	case mcpsession.ExitRunnerPanic:
		return skipCommandFailure(fmt.Errorf("mcp tool runner panicked"))
	case mcpsession.ExitWireBroken:
		return skipCommandFailure(fmt.Errorf("mcp wire broken"))
	default:
		return nil
	}
}

func toolRunner(outer Invocation) mcpsession.Runner {
	return func(ctx context.Context, c mcpsession.Call) mcpsession.Result {
		//nolint:contextcheck // admission builds a throwaway cobra tree; no request ctx to thread into git config
		if err := admitToolArgs(c.Args); err != nil {
			return mcpsession.Result{Stdout: []byte(err.Error() + "\n"), ExitCode: 2}
		}
		var stdout bytes.Buffer
		var stderr bytes.Buffer
		code := Run(ctx, toolInvocation(outer, c.Args, &stdout, &stderr))
		return mcpsession.Result{Stdout: stdout.Bytes(), ExitCode: code}
	}
}

func toolInvocation(outer Invocation, args []string, stdout, stderr *bytes.Buffer) Invocation {
	inv := outer
	inv.Args = args
	inv.TTY = false
	inv.Stdin = strings.NewReader("")
	inv.Stdout = stdout
	inv.Stderr = stderr
	return inv
}

// admitToolArgs refuses Protocol Output, Artifact Output, and a short name
// denylist before Run so those commands never execute inside the session.
func admitToolArgs(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("mcp tool arm requires a non-empty args array")
	}
	switch args[0] {
	case "harness-hook", "hook", "mcp", "worker-init":
		return fmt.Errorf("mcp refuses %q", args[0])
	}

	root := newRootCmd()
	cmd, _, err := root.Find(args)
	if err != nil || cmd == nil {
		return nil
	}
	setFlags := flagNamesSetInArgs(args)
	channel := output.ClassifyFlags(cmd.Annotations, setFlags)
	switch channel {
	case output.ChannelProtocolOutput:
		return fmt.Errorf("mcp refuses Protocol Output command %q", cmd.Name())
	case output.ChannelArtifactOutput:
		return fmt.Errorf("mcp refuses Artifact Output command %q", cmd.Name())
	}
	return nil
}

func flagNamesSetInArgs(args []string) map[string]bool {
	out := make(map[string]bool)
	for _, a := range args {
		if a == "--" {
			break
		}
		if !strings.HasPrefix(a, "--") || a == "--" {
			continue
		}
		name := strings.TrimPrefix(a, "--")
		if i := strings.IndexByte(name, '='); i >= 0 {
			name = name[:i]
		}
		if name != "" {
			out[name] = true
		}
	}
	return out
}
