package ops

import "sync"

// RateLimiter enforces CLI-side rate limits per the spec:
// - Heartbeats: max 1 per minute per issue
// - Creates: max 500 per commit batch
type RateLimiter struct {
	mu              sync.Mutex
	lastHeartbeat   map[string]int64 // issue ID -> last heartbeat timestamp
	createCount     int
	createLimit     int
	heartbeatMinGap int64 // seconds
}

func NewRateLimiter() *RateLimiter {
	return &RateLimiter{
		lastHeartbeat:   make(map[string]int64),
		createLimit:     500,
		heartbeatMinGap: 60,
	}
}

func (r *RateLimiter) AllowHeartbeat(issueID string, nowEpoch int64) bool {
	r.mu.Lock()
	defer r.mu.Unlock()

	last, ok := r.lastHeartbeat[issueID]
	if ok && (nowEpoch-last) < r.heartbeatMinGap {
		return false
	}
	r.lastHeartbeat[issueID] = nowEpoch
	return true
}

func (r *RateLimiter) AllowCreate() bool {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.createCount >= r.createLimit {
		return false
	}
	r.createCount++
	return true
}

func (r *RateLimiter) ResetCreateCount() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.createCount = 0
}
