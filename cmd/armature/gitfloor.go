package main

import (
	"github.com/scullxbones/armature/internal/adapters"
	armerrors "github.com/scullxbones/armature/internal/errors"
	"github.com/spf13/cobra"
)

// gitFloorNextActions is the actionable recovery for a Git-below-floor IO
// failure. Keep as a composite literal at each Wrap site so the ledger AST
// check can resolve the slice (named idents are not accepted).
func gitFloorCommandFailure(err error) error {
	if err == nil {
		return nil
	}
	return armerrors.Wrap(armerrors.CodeIO, err.Error(), []string{
		"upgrade Git to at least 2.34.0",
		"see docs/getting-started.md",
	}, err)
}

// requireMinGitPreRun enforces adapters.MinGitVersion() for commands that
// override the root PersistentPreRunE (worker-init, bootstrap) and for the
// root itself. Platform-protocol commands still go through platformProtocolError.
func requireMinGitPreRun(cmd *cobra.Command) error {
	if err := adapters.RequireMinGit(nil); err != nil {
		return platformProtocolError(cmd, gitFloorCommandFailure(err))
	}
	return nil
}
