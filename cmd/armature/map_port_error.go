package main

import (
	"errors"
	"strings"

	armerrors "github.com/scullxbones/armature/internal/errors"
	"github.com/scullxbones/armature/internal/exitcodes"
	"github.com/scullxbones/armature/internal/output"
	"github.com/spf13/cobra"
)

// mapAgentFacingError presents a port error as a non-GENERAL Command Failure.
// Protocol Output, harness-hook adapter exits, and already-mapped Command
// Failures are returned unchanged. Panics are not recovered.
func mapAgentFacingError(cmd *cobra.Command, err error) error {
	if err == nil {
		return nil
	}
	if ace, ok := errors.AsType[adapterExitError](err); ok {
		return ace
	}
	if pe, ok := errors.AsType[protocolExitError](err); ok {
		return pe
	}
	if staysOnPlatformProtocol(cmd) {
		return skipCommandFailure(err)
	}
	var cf *armerrors.CommandFailure
	if errors.As(err, &cf) {
		return cf
	}
	if isUsageError(err) {
		return armerrors.Wrap(armerrors.CodeUSAGE, err.Error(), []string{"arm --help"}, exitcodes.ExitUsageError.Int(), err)
	}
	code := failureCodeForCommand(cmd)
	return armerrors.Map(code, err.Error(), nextActionsForPortError(err), 1, err)
}

func failureCodeForCommand(cmd *cobra.Command) string {
	name := topLevelCommandUse(cmd)
	if name == "review" {
		return codeReview1
	}
	if name == "" {
		return armerrors.CodeIO
	}
	return armerrors.Prefix(name) + "-1"
}

func topLevelCommandUse(cmd *cobra.Command) string {
	if cmd == nil {
		return ""
	}
	top := cmd
	for top.Parent() != nil && top.Parent().Parent() != nil {
		top = top.Parent()
	}
	if top.Parent() == nil {
		return ""
	}
	fields := strings.Fields(top.Use)
	if len(fields) == 0 {
		return top.Name()
	}
	return fields[0]
}

func nextActionsForPortError(err error) []string {
	msg := err.Error()
	switch {
	case strings.Contains(msg, "ops-worktree-path"),
		strings.Contains(msg, "unmigrated"),
		strings.Contains(msg, "worker not initialized"):
		return []string{"arm bootstrap", "arm worker-init"}
	case strings.Contains(msg, "load snapshot"),
		strings.Contains(msg, "load ops"),
		strings.Contains(msg, "read manifest"):
		return []string{"arm doctor"}
	case strings.Contains(msg, "not found"),
		strings.Contains(msg, "issue ID is required"):
		return []string{"arm list", "arm show"}
	default:
		return []string{"arm doctor"}
	}
}

func isUsageError(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "unknown command") ||
		strings.Contains(msg, "unknown flag") ||
		strings.Contains(msg, "unknown shorthand") ||
		strings.Contains(msg, "required flag") ||
		strings.Contains(msg, "accepts at most") ||
		strings.Contains(msg, "accepts between") ||
		strings.Contains(msg, "arg(s)") ||
		strings.Contains(msg, "requires one of") ||
		strings.Contains(msg, "invalid argument") ||
		strings.Contains(msg, "flag needs an argument") ||
		strings.Contains(msg, "needs an argument")
}

func commandFailureAtPort(err error) *armerrors.CommandFailure {
	mapped := mapAgentFacingError(nil, err)
	var cf *armerrors.CommandFailure
	if errors.As(mapped, &cf) && cf != nil {
		return cf
	}
	return armerrors.New(armerrors.CodeIO, err.Error(), nil, 1)
}

func isProtocolOutputCommand(cmd *cobra.Command) bool {
	if cmd == nil {
		return false
	}
	if staysOnPlatformProtocol(cmd) {
		return true
	}
	return output.Classify(cmd.Annotations) == output.ChannelProtocolOutput
}

func installCommandFailureMapping(root *cobra.Command) {
	if root == nil {
		return
	}
	walkPavedRoadCommands(root, func(cmd *cobra.Command) {
		if cmd.RunE != nil {
			inner := cmd.RunE
			cmd.RunE = func(c *cobra.Command, args []string) error {
				return mapAgentFacingError(c, inner(c, args))
			}
		}
		if cmd.Args != nil {
			inner := cmd.Args
			cmd.Args = func(c *cobra.Command, args []string) error {
				return mapAgentFacingError(c, inner(c, args))
			}
		}
	})
}
