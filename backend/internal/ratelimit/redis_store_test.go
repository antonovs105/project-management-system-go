package ratelimit

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestRedisStoreKeyHashesIdentifier(t *testing.T) {
	store := NewRedisStore(nil, RedisStoreConfig{Prefix: "progo:test"})

	key := store.key(" 192.0.2.10 ")

	assert.True(t, strings.HasPrefix(key, "progo:test:"))
	assert.NotContains(t, key, "192.0.2.10")
	assert.Equal(t, key, store.key("192.0.2.10"))
}

func TestRedisStoreAppliesDefaults(t *testing.T) {
	store := NewRedisStore(nil, RedisStoreConfig{})

	assert.Equal(t, defaultPrefix, store.prefix)
	assert.Equal(t, defaultExpires, store.expiresIn)
	assert.Equal(t, defaultTimeout, store.timeout)
	assert.NotNil(t, store.now)
}

func TestRedisStoreKeepsConfiguredTiming(t *testing.T) {
	now := func() time.Time { return time.Unix(10, 0) }

	store := NewRedisStore(nil, RedisStoreConfig{
		ExpiresIn: 7 * time.Second,
		Timeout:   2 * time.Second,
		Now:       now,
	})

	assert.Equal(t, 7*time.Second, store.expiresIn)
	assert.Equal(t, 2*time.Second, store.timeout)
	assert.Equal(t, now(), store.now())
}
