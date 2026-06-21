// Package ratelimit contains distributed rate limiter stores.
package ratelimit

import (
	"context"
	"crypto/sha256"
	"fmt"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
)

const (
	// defaultPrefix namespaces Redis keys for shared rate limits.
	defaultPrefix = "progo:ratelimit"
	// defaultExpires controls how long idle token buckets stay in Redis.
	defaultExpires = 5 * time.Minute
	// defaultTimeout bounds one Redis limiter operation.
	defaultTimeout = 500 * time.Millisecond
)

// redisTokenBucketScript atomically refills and consumes one token.
var redisTokenBucketScript = redis.NewScript(`
local key = KEYS[1]
local now = tonumber(ARGV[1])
local rate = tonumber(ARGV[2])
local burst = tonumber(ARGV[3])
local expires = tonumber(ARGV[4])

local bucket = redis.call("HMGET", key, "tokens", "updated_at")
local tokens = tonumber(bucket[1])
local updated_at = tonumber(bucket[2])

if tokens == nil or updated_at == nil then
	tokens = burst
	updated_at = now
end

local elapsed = math.max(0, now - updated_at) / 1000
tokens = math.min(burst, tokens + (elapsed * rate))

local allowed = 0
if tokens >= 1 then
	allowed = 1
	tokens = tokens - 1
end

redis.call("HMSET", key, "tokens", tokens, "updated_at", now)
redis.call("EXPIRE", key, expires)
return allowed
`)

// RedisStoreConfig configures a Redis-backed token-bucket limiter.
type RedisStoreConfig struct {
	Prefix            string
	RequestsPerSecond float64
	Burst             int
	ExpiresIn         time.Duration
	Timeout           time.Duration
	Now               func() time.Time
}

// RedisStore implements Echo's RateLimiterStore with shared Redis state.
type RedisStore struct {
	client            *redis.Client
	prefix            string
	requestsPerSecond float64
	burst             int
	expiresIn         time.Duration
	timeout           time.Duration
	now               func() time.Time
}

// NewRedisStore creates a Redis-backed token-bucket store.
func NewRedisStore(client *redis.Client, cfg RedisStoreConfig) *RedisStore {
	prefix := strings.TrimSpace(cfg.Prefix)
	if prefix == "" {
		prefix = defaultPrefix
	}
	expiresIn := cfg.ExpiresIn
	if expiresIn <= 0 {
		expiresIn = defaultExpires
	}
	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = defaultTimeout
	}
	now := cfg.Now
	if now == nil {
		now = time.Now
	}
	return &RedisStore{
		client:            client,
		prefix:            prefix,
		requestsPerSecond: cfg.RequestsPerSecond,
		burst:             cfg.Burst,
		expiresIn:         expiresIn,
		timeout:           timeout,
		now:               now,
	}
}

// Allow consumes one token for identifier.
func (s *RedisStore) Allow(identifier string) (bool, error) {
	if s == nil || s.client == nil {
		return false, fmt.Errorf("redis rate limiter is not configured")
	}
	ctx, cancel := context.WithTimeout(context.Background(), s.timeout)
	defer cancel()

	result, err := redisTokenBucketScript.Run(
		ctx,
		s.client,
		[]string{s.key(identifier)},
		s.now().UTC().UnixMilli(),
		s.requestsPerSecond,
		s.burst,
		int64(s.expiresIn.Seconds()),
	).Int64()
	if err != nil {
		return false, err
	}
	return result == 1, nil
}

// key returns a namespaced hashed key for the caller identifier.
func (s *RedisStore) key(identifier string) string {
	identifier = strings.TrimSpace(identifier)
	if identifier == "" {
		identifier = "unknown"
	}
	sum := sha256.Sum256([]byte(identifier))
	return fmt.Sprintf("%s:%x", s.prefix, sum[:])
}
