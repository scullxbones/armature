//go:build !unix

package main

import "os"

// tryFlock is a no-op fallback for non-unix platforms, where flock(2) has no
// direct equivalent. This repo builds and runs CI on ubuntu-latest only (see
// .github/workflows/ci.yml); this file exists purely so a build targeting an
// unsupported GOOS still compiles. It always reports the lock as acquired and
// therefore provides no actual cross-process exclusion on such platforms.
func tryFlock(_ *os.File) (bool, error) { return true, nil }

// unlockFlock is the no-op counterpart to tryFlock above.
func unlockFlock(_ *os.File) error { return nil }
