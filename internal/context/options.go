package context

import "io/fs"

// Options configures Working Memory rendering. Root is the code-checkout
// directory for Context Files. StateDir is used to infer Root when Root is
// empty. Files, when set, replaces Root/StateDir (fixture collectors).
type Options struct {
	Root         string
	StateDir     string
	Files        fs.FS
	TokenBudget  int
	SkipTruncate bool
}

func (o Options) reader() fileReader {
	if o.Files != nil {
		return fsFileReader{fs: o.Files}
	}
	if root := o.repoRoot(); root != "" {
		return &osFileReader{Root: root}
	}
	return emptyFiles{}
}

func (o Options) repoRoot() string {
	if o.Root != "" {
		return o.Root
	}
	if o.StateDir != "" {
		return inferRepoRoot(o.StateDir)
	}
	return ""
}
