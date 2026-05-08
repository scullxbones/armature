package orchestrate

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/scullxbones/armature/internal/config"
)

// languageDefaults maps a detected language to its built-in default adapter commands.
var languageDefaults = map[string]config.AdapterCommands{
	"go": {
		Build:    "go build ./...",
		Lint:     "golangci-lint run",
		Test:     "go test ./...",
		Coverage: "go test -coverprofile=coverage.out ./...",
		Mutate:   "go-mutesting ./...",
	},
	"node": {
		Build:    "npm run build",
		Lint:     "npm run lint",
		Test:     "npm test",
		Coverage: "npm run coverage",
	},
	"python": {
		Test:     "pytest",
		Lint:     "ruff check .",
		Coverage: "pytest --cov=.",
	},
	"rust": {
		Build:    "cargo build",
		Lint:     "cargo clippy -- -D warnings",
		Test:     "cargo test",
		Coverage: "cargo tarpaulin",
	},
}

// markerOrder defines the probe order for language detection.
// The first matching marker wins.
var markerOrder = []struct {
	file     string
	language string
}{
	{"go.mod", "go"},
	{"package.json", "node"},
	{"pyproject.toml", "python"},
	{"Cargo.toml", "rust"},
}

// DetectAdapters probes dir for known project marker files (go.mod, package.json,
// pyproject.toml, Cargo.toml) and returns an AdapterCommands populated with
// built-in defaults for the detected language. The first matching marker wins.
// An error is returned if dir does not exist or cannot be stat'd.
func DetectAdapters(dir string) (config.AdapterCommands, error) {
	if _, err := os.Stat(dir); err != nil {
		return config.AdapterCommands{}, fmt.Errorf("detect adapters: %w", err)
	}

	for _, m := range markerOrder {
		path := filepath.Join(dir, m.file)
		if _, err := os.Stat(path); err == nil {
			if defaults, ok := languageDefaults[m.language]; ok {
				return defaults, nil
			}
		}
	}

	// No recognised marker found — return empty commands.
	return config.AdapterCommands{}, nil
}

// MergeAdapters returns a new AdapterCommands where each field from override
// wins if it is non-empty, otherwise the corresponding field from base is used.
func MergeAdapters(base, override config.AdapterCommands) config.AdapterCommands {
	merged := base

	if override.Build != "" {
		merged.Build = override.Build
	}
	if override.Lint != "" {
		merged.Lint = override.Lint
	}
	if override.Test != "" {
		merged.Test = override.Test
	}
	if override.Coverage != "" {
		merged.Coverage = override.Coverage
	}
	if override.Mutate != "" {
		merged.Mutate = override.Mutate
	}

	return merged
}
