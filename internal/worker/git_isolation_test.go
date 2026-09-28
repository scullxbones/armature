// Package worker tests isolate GIT_* environment overrides.
package worker

import (
	"os"
	"testing"

	"github.com/scullxbones/armature/internal/gittest"
)

func TestMain(m *testing.M) {
	os.Exit(gittest.Main(m))
}
