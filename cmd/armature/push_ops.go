package main

import (
	"fmt"

	"github.com/scullxbones/armature/internal/adapters"
	"github.com/scullxbones/armature/internal/exitcodes"
	"github.com/spf13/cobra"
)

// newPushOpsCmd creates the push-ops command.
// This command pushes the _armature branch to the remote, making ops logs available to other clones.
// It's called by the post-commit hook after every commit to keep ops data in sync across collaborators.
func newPushOpsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "push-ops",
		Short: "Push ops logs to the remote _armature branch",
		Long: `Push the _armature branch (which contains ops logs) to the remote repository.
This ensures that ops data is shared with other clones and collaborators.
Called by the post-commit hook after each commit.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			repoPath, _ := cmd.Root().PersistentFlags().GetString("repo")
			if repoPath == "" {
				repoPath = "."
			}

			gitClient := adapters.New(repoPath)

			format, _ := cmd.Root().PersistentFlags().GetString("format")

			// Push the _armature branch to the remote.
			//
			// A failed push (no network, no remote, permission denied) is surfaced
			// as a real error and a non-zero exit so a human running `arm push-ops`
			// interactively doesn't get a silent false "success". This is still safe
			// for the post-commit hook: git does not consult a post-commit hook's
			// exit status when deciding whether a commit succeeded, and the hook
			// invokes this command as `arm push-ops 2>/dev/null || true`, so a
			// failed push never blocks or breaks a commit.
			if err := gitClient.Push("_armature"); err != nil {
				if format == "json" || format == "agent" {
					writeJSONError(cmd.ErrOrStderr(), fmt.Sprintf("push-ops: push failed: %v", err), exitcodes.ExitIOError)
				} else {
					_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "push-ops: push failed: %v\n", err)
				}
				return fmt.Errorf("push-ops: push failed: %w", err)
			}

			if format == "json" || format == "agent" {
				// Just output the status, don't fail
				_, _ = fmt.Fprintf(cmd.OutOrStdout(), `{"status":"pushed","branch":"_armature"}`+"\n")
			} else {
				_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Pushed _armature branch to remote\n")
			}
			return nil
		},
	}

	return cmd
}
