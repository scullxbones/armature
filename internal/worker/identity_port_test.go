package worker

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeGitConfigPort struct {
	value    string
	setCalls int
}

func (f *fakeGitConfigPort) Set(key, value string) error {
	f.value = value
	f.setCalls++
	return nil
}

func (f *fakeGitConfigPort) Get(key string) (string, error) {
	return f.value, nil
}

func TestWorkerIdentityUsesGitConfigPort(t *testing.T) {
	port := &fakeGitConfigPort{}

	id, err := InitWorkerWithPort(port)

	require.NoError(t, err)
	assert.NotEmpty(t, id)
	assert.Equal(t, 1, port.setCalls)
}
