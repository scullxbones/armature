package main

import (
	"github.com/scullxbones/armature/internal/adapters"
	armerrors "github.com/scullxbones/armature/internal/errors"
	"github.com/spf13/cobra"
)

func gitFloorCommandFailure(err error) error {
	if err == nil {
		return nil
	}
	return armerrors.Wrap(armerrors.CodeIO, err.Error(), []string{
		"upgrade Git to at least 2.34.0",
		"see docs/getting-started.md",
	}, err)
}

func requireMinGitPreRun(cmd *cobra.Command) error {
	if err := adapters.RequireMinGit(nil); err != nil {
		return platformProtocolError(cmd, gitFloorCommandFailure(err))
	}
	return nil
}
