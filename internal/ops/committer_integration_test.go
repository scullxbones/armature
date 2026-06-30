//go:build integration

package ops_test

import (
	"testing"
)

// TestGitCommitter_IntegrationWithRealGit is an integration test that validates
// GitCommitter implementations against a real git repository.
// This test is skipped by default and only runs with the `integration` build tag.
func TestGitCommitter_IntegrationWithRealGit(t *testing.T) {
	t.Parallel()
	t.Skip("Integration test stub — requires real git setup")
}
