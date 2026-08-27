package store

import (
	"context"
	"time"

	"github.com/redis/go-redis/v9"
)

const certDistributionRateLimitPrefix = "neo-line:cert-dist:"

// RateLimitAllow increments a Redis counter for key and reports whether the
// caller is within limit for the sliding window. When Redis is unreachable the
// caller is allowed (fail-open) and err describes the Redis failure.
func (s *MongoStore) RateLimitAllow(ctx context.Context, key string, limit int, window time.Duration) (allowed bool, err error) {
	if limit <= 0 {
		return true, nil
	}
	fullKey := certDistributionRateLimitPrefix + key
	count, err := s.sessionClient.Incr(ctx, fullKey).Result()
	if err != nil {
		return true, err
	}
	if count == 1 {
		_ = s.sessionClient.Expire(ctx, fullKey, window).Err()
	}
	return count <= int64(limit), nil
}

// IsRedisNil reports whether err is a Redis cache miss.
func IsRedisNil(err error) bool {
	return err == redis.Nil
}
