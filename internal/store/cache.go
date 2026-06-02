package store

import (
	"context"
	"errors"
	"time"

	"github.com/redis/go-redis/v9"
)

// cacheKeyPrefix namespaces ephemeral cache entries in the shared Redis instance.
const cacheKeyPrefix = "neo-line:cache:"

// CacheGet returns the raw bytes stored under key. found is false on a cache miss
// (including when Redis is unreachable), so callers can transparently fall back to
// the source of truth instead of failing the request.
func (s *MongoStore) CacheGet(ctx context.Context, key string) ([]byte, bool, error) {
	data, err := s.sessionClient.Get(ctx, cacheKeyPrefix+key).Bytes()
	if errors.Is(err, redis.Nil) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	return data, true, nil
}

// CacheSet stores data under key with a TTL. A non-nil error is advisory; callers
// treat caching as best-effort and never block on it.
func (s *MongoStore) CacheSet(ctx context.Context, key string, data []byte, ttl time.Duration) error {
	return s.sessionClient.Set(ctx, cacheKeyPrefix+key, data, ttl).Err()
}
