package connectapi

import (
	"testing"
	"time"
)

func TestLoginLimiter(t *testing.T) {
	l := newLoginLimiter()
	now := time.Date(2026, 6, 13, 10, 0, 0, 0, time.UTC)
	key := "user@example.com|1.2.3.4"

	for i := range loginMaxAttempts {
		if !l.allow(key, now) {
			t.Fatalf("attempt %d should be allowed", i)
		}
		l.fail(key, now)
	}
	if l.allow(key, now) {
		t.Fatal("attempt after max failures should be blocked")
	}

	// Window expiry resets the counter.
	if !l.allow(key, now.Add(loginWindow)) {
		t.Fatal("attempt after window should be allowed")
	}
}

func TestLoginLimiterResetOnSuccess(t *testing.T) {
	l := newLoginLimiter()
	now := time.Date(2026, 6, 13, 10, 0, 0, 0, time.UTC)
	key := "user@example.com|1.2.3.4"

	for range loginMaxAttempts {
		l.fail(key, now)
	}
	if l.allow(key, now) {
		t.Fatal("should be blocked after max failures")
	}
	l.reset(key)
	if !l.allow(key, now) {
		t.Fatal("reset should clear failures")
	}
}

func TestLoginLimiterIsolatesKeys(t *testing.T) {
	l := newLoginLimiter()
	now := time.Date(2026, 6, 13, 10, 0, 0, 0, time.UTC)
	for range loginMaxAttempts {
		l.fail("a@example.com|1.1.1.1", now)
	}
	if !l.allow("b@example.com|2.2.2.2", now) {
		t.Fatal("different key should not be throttled")
	}
}
