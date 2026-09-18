package scopematch

// OverlapParityCase is one glob-pair row for claim/validate overlap tests.
type OverlapParityCase struct {
	Name string
	A, B string
	Want bool
}

// OverlapParityCases is the single table claim and validate overlap tests run.
var OverlapParityCases = []OverlapParityCase{
	{"exact match", "internal/claim/a.go", "internal/claim/a.go", true},
	{"glob vs literal in dir", "internal/claim/*.go", "internal/claim/a.go", true},
	{"sibling dir string-prefix, no overlap", "internal/claimx/foo.go", "internal/claim/*.go", false},
	{"no longer overlaps via directory nesting alone", "internal/claim/sub/*.go", "internal/claim/*.go", false},
	{"unrelated dirs", "internal/claim/a.go", "internal/validate/a.go", false},
	{"root-level files, no dir", "a.go", "b.go", false},
	{"explicit doublestar directory glob still overlaps nested file", "internal/claim/**", "internal/claim/sub/a.go", true},
	{"glob-vs-glob same-segment intersection", "src/auth/*.go", "src/auth/login.*", true},
	{"glob-vs-glob distinct literal directories cannot intersect", "src/auth/*.go", "src/billing/*.go", false},
}
