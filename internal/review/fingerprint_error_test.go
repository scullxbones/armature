package review

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestComputeBundleID_UnsupportedPayloadReturnsTypedError_REQ_NOCOMMENTS(t *testing.T) {
	t.Parallel()

	_, err := bundleIDFromPayload(make(chan int))
	require.Error(t, err)
	var typed *BundleIDError
	require.ErrorAs(t, err, &typed)
	assert.Contains(t, typed.Error(), "marshal bundle data")
	assert.Contains(t, typed.Unwrap().Error(), "unsupported type")
}
