package harnesspolicy_test

import (
	"testing"

	"github.com/scullxbones/armature/internal/claim"
	"github.com/scullxbones/armature/internal/harnesspolicy"
	"github.com/stretchr/testify/assert"
)

func TestScopeMatchers_ClaimAndHarnesspolicyAgree_REQ_LNGHZN_S4(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name  string
		scope []string
		path  string
		want  bool
	}{
		{"exact file match", []string{"internal/claim/overlap.go"}, "internal/claim/overlap.go", true},
		{"exact file mismatch", []string{"internal/claim/overlap.go"}, "internal/claim/other.go", false},
		{"trailing slash dir scope covers nested file", []string{"internal/"}, "internal/claim/overlap.go", true},
		{"trailing slash dir scope excludes sibling", []string{"internal/"}, "other/foo.go", false},
		{"doublestar suffix covers nested file", []string{"internal/claim/**"}, "internal/claim/sub/overlap.go", true},
		{"doublestar mid-pattern covers zero segments", []string{"internal/**/api.go"}, "internal/api.go", true},
		{"doublestar mid-pattern covers one segment", []string{"internal/**/api.go"}, "internal/foo/api.go", true},
		{"doublestar mid-pattern excludes unrelated file", []string{"internal/**/api.go"}, "internal/foo/other.go", false},
		{"dot root scope covers any path", []string{"."}, "cmd/armature/main.go", true},
		{"leading ./ prefix on scope entry normalizes", []string{"./internal/claim/**"}, "internal/claim/overlap.go", true},
		{"single-star glob within a dir", []string{"internal/claim/*.go"}, "internal/claim/overlap.go", true},
		{"single-star glob does not cross dir boundary", []string{"internal/claim/*.go"}, "internal/claim/sub/overlap.go", false},
		{"case-sensitive mismatch", []string{"internal/claim/**"}, "internal/Claim/overlap.go", false},
		{"(new) annotation stripped before matching", []string{"internal/foo.go (new)"}, "internal/foo.go", true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			claimAllows, _ := claim.IsWithinScope([]string{tc.path}, tc.scope)

			policy := harnesspolicy.NewScopePolicyWithRoot(tc.scope, "")
			policyResult := policy.CheckPaths([]string{tc.path})

			assert.Equal(t, tc.want, claimAllows, "claim.IsWithinScope(%q, %v)", tc.path, tc.scope)
			assert.Equal(t, tc.want, policyResult.Allowed, "harnesspolicy.ScopePolicy.CheckPaths(%q) with scope %v", tc.path, tc.scope)
			assert.Equal(t, claimAllows, policyResult.Allowed,
				"claim and harnesspolicy scope matchers disagree on path %q with scope %v", tc.path, tc.scope)
		})
	}
}
