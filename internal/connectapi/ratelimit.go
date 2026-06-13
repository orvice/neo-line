package connectapi

import (
	"sync"
	"time"
)

const (
	// loginMaxAttempts is how many failed logins a single key may accumulate
	// within loginWindow before further attempts are rejected.
	loginMaxAttempts = 5
	loginWindow      = 15 * time.Minute
)

// loginLimiter throttles repeated failed logins per key (email+IP) to slow down
// credential brute-forcing. Successful logins clear the key. State is in-memory
// and best-effort: it does not survive restarts and is not shared across nodes.
type loginLimiter struct {
	mu      sync.Mutex
	entries map[string]*loginAttempt
}

type loginAttempt struct {
	count       int
	windowStart time.Time
}

func newLoginLimiter() *loginLimiter {
	return &loginLimiter{entries: make(map[string]*loginAttempt)}
}

// allow reports whether a login attempt for key may proceed. Expired windows
// reset automatically. It does not increment the failure count; callers record
// failures via fail.
func (l *loginLimiter) allow(key string, now time.Time) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	entry, ok := l.entries[key]
	if !ok {
		return true
	}
	if now.Sub(entry.windowStart) >= loginWindow {
		delete(l.entries, key)
		return true
	}
	return entry.count < loginMaxAttempts
}

// fail records a failed attempt for key, starting a fresh window when needed.
func (l *loginLimiter) fail(key string, now time.Time) {
	l.mu.Lock()
	defer l.mu.Unlock()
	entry, ok := l.entries[key]
	if !ok || now.Sub(entry.windowStart) >= loginWindow {
		l.entries[key] = &loginAttempt{count: 1, windowStart: now}
		return
	}
	entry.count++
}

// reset clears any recorded failures for key after a successful login.
func (l *loginLimiter) reset(key string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.entries, key)
}
