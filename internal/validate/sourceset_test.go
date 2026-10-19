package validate

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSourceSetZeroValueSkipsCitationChecks(t *testing.T) {
	t.Parallel()
	var s SourceSet
	assert.False(t, s.Loaded())
	assert.NoError(t, s.ParseError())
	assert.False(t, s.Contains("src"))
}

func TestKnownSourcesTreatsNilAsEmptyLoadedSet(t *testing.T) {
	t.Parallel()
	s := KnownSources(nil)
	assert.True(t, s.Loaded())
	assert.NoError(t, s.ParseError())
	assert.False(t, s.Contains("src"))
}

func TestUnreadableSourcesPreservesParseError(t *testing.T) {
	t.Parallel()
	want := errors.New("json: cannot unmarshal number")
	s := UnreadableSources(want)
	assert.True(t, s.Loaded())
	require.EqualError(t, s.ParseError(), want.Error())
	assert.False(t, s.Contains("src"))
}

func TestKnownSourcesContainsOnlyProvidedIDs(t *testing.T) {
	t.Parallel()
	s := KnownSources(map[string]struct{}{"src-1": {}})
	assert.True(t, s.Contains("src-1"))
	assert.False(t, s.Contains("src-2"))
}
