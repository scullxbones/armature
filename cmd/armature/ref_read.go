package main

import (
	"fmt"

	"github.com/scullxbones/armature/internal/externalref"
	"github.com/scullxbones/armature/internal/materialize"
	"github.com/spf13/cobra"
)

func newRefReadCmd() *cobra.Command {
	var urlFlag string

	cmd := &cobra.Command{
		Use:   "ref-read [issue-id]",
		Short: "Print the title and body of a live host ref",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			issueID := args[0]
			store := newSnapshotStore(currentCtx(cmd))
			snap, err := store.LoadReadOnly(cmd.Context())
			if err != nil {
				return fmt.Errorf("load snapshot: %w", err)
			}
			emitSnapWarnings(cmd.ErrOrStderr(), snap.Warnings)

			issuePtr, ok := snap.Issues[issueID]
			if !ok || issuePtr == nil {
				return fmt.Errorf("issue %q not found", issueID)
			}

			hostURL, err := resolveLiveHostRefURL(issuePtr.HostRefs, urlFlag)
			if err != nil {
				return err
			}
			ghIssue, err := externalref.ParseGitHubIssueURL(hostURL)
			if err != nil {
				return err
			}

			env := envFrom(cmd)
			text, err := externalref.FetchGitHubIssue(cmd.Context(), ghIssue, env.Lookup)
			if err != nil {
				return err
			}

			writeCommandResult(cmd, map[string]string{
				"url":   text.URL,
				"title": text.Title,
				"body":  text.Body,
			}, "%s\n\n%s\n", text.Title, text.Body)
			return nil
		},
	}

	cmd.Flags().StringVar(&urlFlag, "url", "", "live host URL to read (default: current)")
	return cmd
}

func resolveLiveHostRefURL(refs []materialize.HostRef, rawURL string) (string, error) {
	live := materialize.LiveHostRefs(refs)
	if rawURL == "" {
		if len(live) == 0 {
			return "", fmt.Errorf("issue has no current host ref")
		}
		return live[len(live)-1].URL, nil
	}
	parsed, err := externalref.ParseGitHubIssueURL(rawURL)
	if err != nil {
		return "", err
	}
	canonical := parsed.URL()
	for _, ref := range live {
		if ref.URL == canonical {
			return canonical, nil
		}
	}
	return "", fmt.Errorf("host ref %q is not a live name on this issue", canonical)
}
