package review

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseActivityLogBytes_MalformedInputs_REQ_NOCOMMENTS(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		line    string
		wantSub string
	}{
		{
			name:    "invalid JSON",
			line:    `this is a completely malformed line`,
			wantSub: "activity log line 0",
		},
		{
			name:    "empty object zero-fills command",
			line:    `{}`,
			wantSub: "missing command",
		},
		{
			name:    "command omitted",
			line:    `{"timestamp":"t","exit_code":0,"exit_code_known":true,"head_sha":"h","output_hash":"o"}`,
			wantSub: "missing command",
		},
		{
			name:    "known exit without exit_code",
			line:    `{"command":"make test","exit_code_known":true,"head_sha":"h"}`,
			wantSub: "exit_code is omitted",
		},
		{
			name:    "known exit with null exit_code",
			line:    `{"command":"make test","exit_code":null,"exit_code_known":true,"head_sha":"h"}`,
			wantSub: "activity log line 0: exit_code must be an integer, not null",
		},
		{
			name:    "known exit with string exit_code",
			line:    `{"command":"make test","exit_code":"0","exit_code_known":true,"head_sha":"h"}`,
			wantSub: "activity log line 0: exit_code must be an integer",
		},
		{
			name:    "known exit with float exit_code",
			line:    `{"command":"make test","exit_code":0.5,"exit_code_known":true,"head_sha":"h"}`,
			wantSub: "activity log line 0: exit_code must be an integer",
		},
		{
			name:    "truncated JSON object",
			line:    `{"command":"make test"`,
			wantSub: "activity log line 0",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			_, err := parseActivityLogBytes([]byte(tt.line + "\n"))
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantSub)
		})
	}
}

func TestParseActivityLogBytes_ValidKnownExitStillParses(t *testing.T) {
	t.Parallel()
	entries, err := parseActivityLogBytes([]byte(
		`{"timestamp":"t","command":"make test","exit_code":0,"exit_code_known":true,"head_sha":"h","output_hash":"o"}` + "\n",
	))
	require.NoError(t, err)
	require.Len(t, entries, 1)
	assert.Equal(t, "make test", entries[0].Command)
	assert.True(t, entries[0].ExitCodeKnown)
	assert.Equal(t, 0, entries[0].ExitCode)
	assert.False(t, strings.Contains(entries[0].Command, "\x00"))
}
