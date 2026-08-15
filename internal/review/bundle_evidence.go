package review

import (
	"fmt"
	"path/filepath"

	"github.com/scullxbones/armature/internal/ops"
)

// AttachGateEvidence copies gate-evidence ops from the repo's worker logs onto the bundle.
func AttachGateEvidence(bundle *ReviewBundle, issuesDir string) error {
	if bundle == nil {
		return fmt.Errorf("attach gate evidence: bundle is nil")
	}
	if issuesDir == "" {
		return nil
	}
	opsDir := filepath.Join(issuesDir, "ops")
	evs, err := ops.ReadAllGateEvidence(opsDir)
	if err != nil {
		return fmt.Errorf("read gate evidence: %w", err)
	}
	if len(evs) == 0 {
		return nil
	}
	bundle.GateEvidence = evs
	return nil
}
