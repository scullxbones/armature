package ops

import (
	"fmt"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/scullxbones/armature/internal/adapters"
)

// PendingPushTracker tracks how many low-stakes ops are pending a push.
type PendingPushTracker interface {
	// Increment adds one to the pending count and returns the new total.
	Increment() (int, error)
	// Reset sets the pending count back to zero.
	Reset() error
}

// NoTracker is a no-op PendingPushTracker (used in single-branch mode).
type NoTracker struct{}

func (NoTracker) Increment() (int, error) { return 0, nil }
func (NoTracker) Reset() error            { return nil }
func (NoTracker) Count() (int, error)     { return 0, nil }

type filePushTracker struct {
	Path string
}

func NewFilePushTracker(stateDir string) *filePushTracker {
	return &filePushTracker{
		Path: filepath.Join(stateDir, "pending-push-count"),
	}
}

func (f *filePushTracker) Count() (int, error) {
	data, err := adapters.ReadFile(f.Path)
	if err != nil {
		if data == nil {
			return 0, nil
		}
		return 0, fmt.Errorf("read pending-push-count: %w", err)
	}
	n, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil {
		return 0, nil
	}
	return n, nil
}

func (f *filePushTracker) Increment() (int, error) {
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

func (f *filePushTracker) Reset() error {
	return f.write(0)
}

func (f *filePushTracker) write(n int) error {
	if err := adapters.MkdirAll(filepath.Dir(f.Path), 0755); err != nil {
		return fmt.Errorf("mkdir pending-push-count: %w", err)
	}
	return adapters.WriteFile(f.Path, []byte(strconv.Itoa(n)), 0644)
}
