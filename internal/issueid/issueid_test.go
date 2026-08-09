package issueid

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidate(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name string
		id   string
		want string
	}{
		{name: "ordinary durable ID", id: "LNGHZN-S5-T10"},
		{name: "empty", id: "", want: "is required"},
		{name: "absolute", id: "/tmp/issue", want: "absolute path"},
		{name: "slash", id: "team/task", want: "path separators"},
		{name: "backslash", id: "team\\task", want: "path separators"},
		{name: "dot", id: ".", want: "path component"},
		{name: "dot dot", id: "..", want: "path component"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			err := Validate(tc.id)
			if tc.want == "" {
				assert.NoError(t, err)
				return
			}
			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.want)
		})
	}
}
