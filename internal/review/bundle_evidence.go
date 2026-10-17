package review

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
)

func AttachGateEvidence(bundle *ReviewBundle, records []byte) error {
	if bundle == nil {
		return fmt.Errorf("attach gate evidence: bundle is nil")
	}
	if bundle.Delivery.HeadSHA == "" || len(records) == 0 || string(records) == "null" {
		return nil
	}
	var evs []gateEvidence
	if err := json.Unmarshal(records, &evs); err != nil {
		return fmt.Errorf("attach gate evidence: %w", err)
	}
	var kept []gateEvidence
	for _, ev := range evs {
		if ev.HeadSHA == bundle.Delivery.HeadSHA && ev.citable() {
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

func validateGateEvidenceLogs(evidence []gateEvidence) error {
	for i, ev := range evidence {
		if ev.OutputHash == "" {
			return fmt.Errorf("gate evidence %d: missing output_hash", i)
		}
		if ev.LogPath == "" {
			return fmt.Errorf("gate evidence %d: missing log_path", i)
		}
		data, err := readLogFile(ev.LogPath)
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

func readLogFile(path string) ([]byte, error) {
	return os.ReadFile(path) //nolint:gosec
}
