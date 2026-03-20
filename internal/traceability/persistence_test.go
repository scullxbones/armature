package traceability_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/scullxbones/trellis/internal/traceability"
)

func TestWriteAndRead_RoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "traceability.json")

	cov := traceability.Coverage{
		TotalNodes:  4,
		CitedNodes:  3,
		CoveragePct: 75.0,
		Uncited:     []string{"ISSUE-2"},
	}

	if err := traceability.Write(path, cov); err != nil {
		t.Fatalf("Write returned error: %v", err)
	}

	got, err := traceability.Read(path)
	if err != nil {
		t.Fatalf("Read returned error: %v", err)
	}
	if got.TotalNodes != cov.TotalNodes {
		t.Errorf("TotalNodes: got %d, want %d", got.TotalNodes, cov.TotalNodes)
	}
	if got.CitedNodes != cov.CitedNodes {
		t.Errorf("CitedNodes: got %d, want %d", got.CitedNodes, cov.CitedNodes)
	}
	if got.CoveragePct != cov.CoveragePct {
		t.Errorf("CoveragePct: got %f, want %f", got.CoveragePct, cov.CoveragePct)
	}
}

func TestRead_MissingFile_ReturnsZero(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "nonexistent.json")

	got, err := traceability.Read(path)
	if err != nil {
		t.Fatalf("Read of missing file returned error: %v", err)
	}
	if got.TotalNodes != 0 || got.CitedNodes != 0 {
		t.Errorf("expected zero Coverage for missing file, got %+v", got)
	}
}

func TestRead_InvalidJSON_ReturnsError(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bad.json")

	if err := os.WriteFile(path, []byte("not-json{{{"), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	_, err := traceability.Read(path)
	if err == nil {
		t.Fatal("expected error for invalid JSON, got nil")
	}
}
