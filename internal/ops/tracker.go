package ops

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// PendingPushTracker tracks how many low-stakes ops are pending a push.
type PendingPushTracker interface {
	// Increment adds one to the pending count and returns the new total.
	Increment() (int, error)
	// Reset sets the pending count back to zero.
	Reset() error
	// Count returns the current pending count.
	Count() (int, error)
}

// NoTracker is a no-op PendingPushTracker (used in single-branch mode).
type NoTracker struct{}

func (NoTracker) Increment() (int, error) { return 0, nil }
func (NoTracker) Reset() error            { return nil }
func (NoTracker) Count() (int, error)     { return 0, nil }

// FilePushTracker persists the pending push count to a file at
// .issues/state/pending-push-count.
type FilePushTracker struct {
	Path string
}

// NewFilePushTracker creates a FilePushTracker writing to stateDir/pending-push-count.
func NewFilePushTracker(stateDir string) *FilePushTracker {
	return &FilePushTracker{
		Path: filepath.Join(stateDir, "pending-push-count"),
	}
}

func (f *FilePushTracker) Count() (int, error) {
	data, err := os.ReadFile(f.Path)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, nil
		}
		return 0, fmt.Errorf("read pending-push-count: %w", err)
	}
	n, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil {
		return 0, nil // treat corrupt file as 0
	}
	return n, nil
}

func (f *FilePushTracker) Increment() (int, error) {
	n, err := f.Count()
	if err != nil {
		return 0, err
	}
	n++
	if err := f.write(n); err != nil {
		return 0, err
	}
	return n, nil
}

func (f *FilePushTracker) Reset() error {
	return f.write(0)
}

func (f *FilePushTracker) write(n int) error {
	if err := os.MkdirAll(filepath.Dir(f.Path), 0755); err != nil {
		return fmt.Errorf("mkdir pending-push-count: %w", err)
	}
	return os.WriteFile(f.Path, []byte(strconv.Itoa(n)), 0644)
}
