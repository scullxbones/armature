package ops

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDecodeScope_REQ_MATENC_S1_T1(t *testing.T) {
	t.Parallel()

	t.Run("legacy comma-space joined entry splits", func(t *testing.T) {
		t.Parallel()
		got := DecodeScope([]string{"cmd/a.go, cmd/b.go, cmd/c.go"})
		assert.Equal(t, []string{"cmd/a.go", "cmd/b.go", "cmd/c.go"}, got)
	})

	t.Run("already-split entries are unchanged", func(t *testing.T) {
		t.Parallel()
		got := DecodeScope([]string{"src/auth/**", "src/session/**"})
		assert.Equal(t, []string{"src/auth/**", "src/session/**"}, got)
	})

	t.Run("comma without space is not a split", func(t *testing.T) {
		t.Parallel()
		joined := []string{"a.go,b.go"}
		assert.Equal(t, []string{"a.go,b.go"}, DecodeScope(joined))
	})

	t.Run("empty and whitespace entries are dropped", func(t *testing.T) {
		t.Parallel()
		got := DecodeScope([]string{" ", "src/auth/**", "", "src/db/**, src/cache/**"})
		assert.Equal(t, []string{"src/auth/**", "src/db/**", "src/cache/**"}, got)
	})

	t.Run("empty parts after split are dropped", func(t *testing.T) {
		t.Parallel()
		got := DecodeScope([]string{"a.go, , b.go"})
		assert.Equal(t, []string{"a.go", "b.go"}, got)
	})

	t.Run("nil and empty inputs yield an empty slice", func(t *testing.T) {
		t.Parallel()
		assert.Empty(t, DecodeScope(nil))
		assert.Empty(t, DecodeScope([]string{}))
	})

	t.Run("ParseLine does not mutate payload scope", func(t *testing.T) {
		t.Parallel()
		line := []byte(`["create","T1",1,"w1",{"title":"t","type":"task","scope":["a.go, b.go"]}]`)
		op, err := ParseLine(line)
		require.NoError(t, err)
		assert.Equal(t, []string{"a.go, b.go"}, op.Payload.Scope)
		assert.Equal(t, []string{"a.go", "b.go"}, DecodeScope(op.Payload.Scope))
		assert.Equal(t, []string{"a.go, b.go"}, op.Payload.Scope, "DecodeScope must not mutate the payload")
	})
}
