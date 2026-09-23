package doctor

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestParseGitStatusPorcelain(t *testing.T) {
	t.Parallel()
	got := parseGitStatusPorcelain([]byte(" M internal/foo.go\n?? stray.bin\nR  old.go -> new.go\nD  gone.go\n?? \"quoted.go\"\nXY\n"))
	assert.Equal(t, []string{"internal/foo.go", "stray.bin", "new.go", "gone.go", "quoted.go"}, got)
	assert.Empty(t, parseGitStatusPorcelain(nil))
}
