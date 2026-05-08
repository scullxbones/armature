package orchestrate_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/scullxbones/armature/internal/config"
	"github.com/scullxbones/armature/internal/orchestrate"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// tempDir creates a temporary directory and returns its path.
// It registers cleanup via t.Cleanup.
func tempDir(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp("", "framework-test-*")
	require.NoError(t, err)
	t.Cleanup(func() {
		if err := os.RemoveAll(dir); err != nil {
			t.Logf("cleanup: failed to remove temp dir %s: %v", dir, err)
		}
	})
	return dir
}

// touch creates an empty file at path inside dir.
func touch(t *testing.T, dir, name string) {
	t.Helper()
	path := filepath.Join(dir, name)
	f, err := os.Create(path)
	require.NoError(t, err)
	require.NoError(t, f.Close())
}

// --- DetectAdapters ---

func TestDetectAdapters_GoProject(t *testing.T) {
	dir := tempDir(t)
	touch(t, dir, "go.mod")

	got, err := orchestrate.DetectAdapters(dir)
	require.NoError(t, err)

	assert.NotEmpty(t, got.Build, "go project should have a default Build command")
	assert.NotEmpty(t, got.Lint, "go project should have a default Lint command")
	assert.NotEmpty(t, got.Test, "go project should have a default Test command")
}

func TestDetectAdapters_NodeProject(t *testing.T) {
	dir := tempDir(t)
	touch(t, dir, "package.json")

	got, err := orchestrate.DetectAdapters(dir)
	require.NoError(t, err)

	assert.NotEmpty(t, got.Build, "node project should have a default Build command")
	assert.NotEmpty(t, got.Test, "node project should have a default Test command")
}

func TestDetectAdapters_PythonProject(t *testing.T) {
	dir := tempDir(t)
	touch(t, dir, "pyproject.toml")

	got, err := orchestrate.DetectAdapters(dir)
	require.NoError(t, err)

	assert.NotEmpty(t, got.Test, "python project should have a default Test command")
}

func TestDetectAdapters_RustProject(t *testing.T) {
	dir := tempDir(t)
	touch(t, dir, "Cargo.toml")

	got, err := orchestrate.DetectAdapters(dir)
	require.NoError(t, err)

	assert.NotEmpty(t, got.Build, "rust project should have a default Build command")
	assert.NotEmpty(t, got.Test, "rust project should have a default Test command")
}

func TestDetectAdapters_UnknownProject(t *testing.T) {
	dir := tempDir(t)
	// No marker files present

	got, err := orchestrate.DetectAdapters(dir)
	require.NoError(t, err)

	// Unknown projects return empty commands (no defaults guessed)
	assert.Empty(t, got.Build)
	assert.Empty(t, got.Test)
}

func TestDetectAdapters_NonExistentDir(t *testing.T) {
	_, err := orchestrate.DetectAdapters("/no/such/directory/xyz123")
	assert.Error(t, err, "should return error for nonexistent directory")
}

func TestDetectAdapters_GoDefaults(t *testing.T) {
	dir := tempDir(t)
	touch(t, dir, "go.mod")

	got, err := orchestrate.DetectAdapters(dir)
	require.NoError(t, err)

	assert.Contains(t, got.Build, "go")
	assert.Contains(t, got.Test, "go")
}

func TestDetectAdapters_RustDefaults(t *testing.T) {
	dir := tempDir(t)
	touch(t, dir, "Cargo.toml")

	got, err := orchestrate.DetectAdapters(dir)
	require.NoError(t, err)

	assert.Contains(t, got.Build, "cargo")
	assert.Contains(t, got.Test, "cargo")
}

func TestDetectAdapters_NodeDefaults(t *testing.T) {
	dir := tempDir(t)
	touch(t, dir, "package.json")

	got, err := orchestrate.DetectAdapters(dir)
	require.NoError(t, err)

	assert.Contains(t, got.Test, "npm")
}

func TestDetectAdapters_PythonDefaults(t *testing.T) {
	dir := tempDir(t)
	touch(t, dir, "pyproject.toml")

	got, err := orchestrate.DetectAdapters(dir)
	require.NoError(t, err)

	assert.Contains(t, got.Test, "pytest")
}

// Priority: go.mod wins when multiple markers exist (first-match semantics)
func TestDetectAdapters_GoWinsOverNode(t *testing.T) {
	dir := tempDir(t)
	touch(t, dir, "go.mod")
	touch(t, dir, "package.json")

	got, err := orchestrate.DetectAdapters(dir)
	require.NoError(t, err)

	// Should be Go defaults
	assert.Contains(t, got.Build, "go")
}

// --- MergeAdapters ---

func TestMergeAdapters_OverrideWins(t *testing.T) {
	base := config.AdapterCommands{
		Build: "go build ./...",
		Lint:  "golangci-lint run",
		Test:  "go test ./...",
	}
	override := config.AdapterCommands{
		Build: "make build",
		Test:  "make test",
	}

	merged := orchestrate.MergeAdapters(base, override)

	assert.Equal(t, "make build", merged.Build, "override Build should win")
	assert.Equal(t, "golangci-lint run", merged.Lint, "base Lint should be kept when override is empty")
	assert.Equal(t, "make test", merged.Test, "override Test should win")
}

func TestMergeAdapters_EmptyOverride(t *testing.T) {
	base := config.AdapterCommands{
		Build:    "go build ./...",
		Lint:     "golangci-lint run",
		Test:     "go test ./...",
		Coverage: "go test -cover ./...",
		Mutate:   "go-mutesting ./...",
	}
	override := config.AdapterCommands{} // all empty

	merged := orchestrate.MergeAdapters(base, override)

	assert.Equal(t, base.Build, merged.Build)
	assert.Equal(t, base.Lint, merged.Lint)
	assert.Equal(t, base.Test, merged.Test)
	assert.Equal(t, base.Coverage, merged.Coverage)
	assert.Equal(t, base.Mutate, merged.Mutate)
}

func TestMergeAdapters_EmptyBase(t *testing.T) {
	base := config.AdapterCommands{}
	override := config.AdapterCommands{
		Build: "npm run build",
		Test:  "npm test",
	}

	merged := orchestrate.MergeAdapters(base, override)

	assert.Equal(t, "npm run build", merged.Build)
	assert.Equal(t, "npm test", merged.Test)
	assert.Empty(t, merged.Lint)
}

func TestMergeAdapters_AllFields(t *testing.T) {
	base := config.AdapterCommands{
		Build:    "base-build",
		Lint:     "base-lint",
		Test:     "base-test",
		Coverage: "base-coverage",
		Mutate:   "base-mutate",
	}
	override := config.AdapterCommands{
		Build:    "override-build",
		Lint:     "override-lint",
		Test:     "override-test",
		Coverage: "override-coverage",
		Mutate:   "override-mutate",
	}

	merged := orchestrate.MergeAdapters(base, override)

	assert.Equal(t, "override-build", merged.Build)
	assert.Equal(t, "override-lint", merged.Lint)
	assert.Equal(t, "override-test", merged.Test)
	assert.Equal(t, "override-coverage", merged.Coverage)
	assert.Equal(t, "override-mutate", merged.Mutate)
}

func TestMergeAdapters_PartialOverride(t *testing.T) {
	base := config.AdapterCommands{
		Build:    "go build ./...",
		Lint:     "golangci-lint run",
		Test:     "go test ./...",
		Coverage: "go test -cover ./...",
		Mutate:   "go-mutesting ./...",
	}
	override := config.AdapterCommands{
		Lint: "custom-lint",
	}

	merged := orchestrate.MergeAdapters(base, override)

	assert.Equal(t, base.Build, merged.Build, "non-overridden Build keeps base")
	assert.Equal(t, "custom-lint", merged.Lint, "overridden Lint uses override")
	assert.Equal(t, base.Test, merged.Test, "non-overridden Test keeps base")
	assert.Equal(t, base.Coverage, merged.Coverage, "non-overridden Coverage keeps base")
	assert.Equal(t, base.Mutate, merged.Mutate, "non-overridden Mutate keeps base")
}

func TestMergeAdapters_Idempotent(t *testing.T) {
	base := config.AdapterCommands{
		Build: "go build ./...",
		Test:  "go test ./...",
	}

	merged1 := orchestrate.MergeAdapters(base, config.AdapterCommands{})
	merged2 := orchestrate.MergeAdapters(merged1, config.AdapterCommands{})

	assert.Equal(t, merged1, merged2, "merging with empty override is idempotent")
}
