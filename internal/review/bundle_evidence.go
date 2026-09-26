package review

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"path/filepath"

	"github.com/scullxbones/armature/internal/adapters"
	"github.com/scullxbones/armature/internal/ops"
)

// AttachGateEvidence copies citable gate-evidence ops that match the delivery
// head onto the bundle. Non-citable or wrong-SHA records are omitted; an
// empty delivery head attaches nothing.
func AttachGateEvidence(bundle *ReviewBundle, issuesDir string) error {
	if bundle == nil {
		return fmt.Errorf("attach gate evidence: bundle is nil")
	}
	if issuesDir == "" || bundle.Delivery.HeadSHA == "" {
		return nil
	}
	opsDir := filepath.Join(issuesDir, "ops")
	evs, err := ops.ReadAllGateEvidence(opsDir)
	if err != nil {
		return fmt.Errorf("read gate evidence: %w", err)
	}
	var kept []ops.GateEvidence
	for _, ev := range evs {
		if ev.HeadSHA == bundle.Delivery.HeadSHA && ev.Citable() {
			kept = append(kept, ev)
		}
	}
	if len(kept) == 0 {
		return nil
	}
	bundle.GateEvidence = kept
	id, err := ComputeBundleID(*bundle)
	if err != nil {
		return err
	}
	bundle.BundleID = id
	return nil
}

// ValidateGateEvidenceLogs re-reads each attached evidence log and checks
// its SHA-256 against the recorded output_hash. Empty hashes and missing
// files fail closed: record must not attest unverified evidence.
func ValidateGateEvidenceLogs(evidence []ops.GateEvidence) error {
	for i, ev := range evidence {
		if ev.OutputHash == "" {
			return fmt.Errorf("gate evidence %d: missing output_hash", i)
		}
		if ev.LogPath == "" {
			return fmt.Errorf("gate evidence %d: missing log_path", i)
		}
		data, err := adapters.ReadFile(ev.LogPath)
		if err != nil {
			return fmt.Errorf("gate evidence %d: read log %s: %w", i, ev.LogPath, err)
		}
		got := hex.EncodeToString(sha256Sum(data))
		if got != ev.OutputHash {
			return fmt.Errorf("gate evidence %d: output hash mismatch", i)
		}
	}
	return nil
}

func sha256Sum(b []byte) []byte {
	sum := sha256.Sum256(b)
	return sum[:]
}
