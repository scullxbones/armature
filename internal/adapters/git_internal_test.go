package adapters

import "testing"

func TestIsBenignEmptyRepoRmError(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name   string
		output string
		want   bool
	}{
		{
			name:   "empty repo pathspec message",
			output: "fatal: pathspec '.' did not match any files\n",
			want:   true,
		},
		{
			name:   "permission denied is not benign",
			output: "error: unable to unlink 'foo': Permission denied\n",
			want:   false,
		},
		{
			name:   "unrelated failure is not benign",
			output: "fatal: not a git repository\n",
			want:   false,
		},
		{
			name:   "empty output is not benign",
			output: "",
			want:   false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := isBenignEmptyRepoRmError([]byte(tc.output))
			if got != tc.want {
				t.Errorf("isBenignEmptyRepoRmError(%q) = %v, want %v", tc.output, got, tc.want)
			}
		})
	}
}

func TestCmdContextSetsLocaleToC(t *testing.T) {
	t.Parallel()

	c := New(t.TempDir())
	cmd := c.cmdContext(t.Context(), "status")

	var gotLCAll, gotLang bool
	for _, env := range cmd.Env {
		switch env {
		case "LC_ALL=C":
			gotLCAll = true
		case "LANG=C":
			gotLang = true
		}
	}

	if !gotLCAll || !gotLang {
		t.Fatalf("cmdContext env missing locale overrides: LC_ALL=C=%v LANG=C=%v", gotLCAll, gotLang)
	}
}
