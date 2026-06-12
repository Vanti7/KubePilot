package store

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
)

// ErrCacheMiss is returned by Cache.Get when the key is absent or expired.
var ErrCacheMiss = errors.New("cache miss")

// Cache is the minimal key/value cache contract used by the watchers. It is
// implemented by both Redis (production) and an in-process map (local mode),
// so the rest of the application never depends on Redis directly.
type Cache interface {
	// Get returns the stored value or ErrCacheMiss if absent/expired.
	Get(ctx context.Context, key string) (string, error)
	// Set stores value under key with a time-to-live.
	Set(ctx context.Context, key string, value []byte, ttl time.Duration) error
	// Ping verifies the cache is reachable.
	Ping(ctx context.Context) error
	// Close releases any underlying resources.
	Close() error
}

// ---------------------------------------------------------------------------
// Redis-backed cache
// ---------------------------------------------------------------------------

// RedisCache implements Cache on top of a go-redis client.
type RedisCache struct {
	client *redis.Client
}

// NewRedisCache parses a redis URL, opens a client and verifies connectivity.
func NewRedisCache(url string) (*RedisCache, error) {
	opt, err := redis.ParseURL(url)
	if err != nil {
		return nil, err
	}
	client := redis.NewClient(opt)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := client.Ping(ctx).Err(); err != nil {
		return nil, err
	}
	return &RedisCache{client: client}, nil
}

func (c *RedisCache) Get(ctx context.Context, key string) (string, error) {
	val, err := c.client.Get(ctx, key).Result()
	if err == redis.Nil {
		return "", ErrCacheMiss
	}
	return val, err
}

func (c *RedisCache) Set(ctx context.Context, key string, value []byte, ttl time.Duration) error {
	return c.client.Set(ctx, key, value, ttl).Err()
}

func (c *RedisCache) Ping(ctx context.Context) error {
	return c.client.Ping(ctx).Err()
}

func (c *RedisCache) Close() error {
	return c.client.Close()
}

// ---------------------------------------------------------------------------
// In-memory cache (local / single-binary mode)
// ---------------------------------------------------------------------------

type memoryEntry struct {
	value     []byte
	expiresAt time.Time // zero == no expiry
}

// MemoryCache is a goroutine-safe in-process cache with per-key TTL. It is used
// in local mode so the single binary needs no Redis. A background janitor
// evicts expired keys; expiry is also enforced lazily on Get.
type MemoryCache struct {
	mu     sync.RWMutex
	items  map[string]memoryEntry
	stopCh chan struct{}
	once   sync.Once
}

// NewMemoryCache creates an empty in-memory cache and starts its janitor.
func NewMemoryCache() *MemoryCache {
	c := &MemoryCache{
		items:  make(map[string]memoryEntry),
		stopCh: make(chan struct{}),
	}
	go c.janitor()
	return c
}

func (c *MemoryCache) Get(_ context.Context, key string) (string, error) {
	c.mu.RLock()
	entry, ok := c.items[key]
	c.mu.RUnlock()
	if !ok {
		return "", ErrCacheMiss
	}
	if !entry.expiresAt.IsZero() && time.Now().After(entry.expiresAt) {
		c.mu.Lock()
		delete(c.items, key)
		c.mu.Unlock()
		return "", ErrCacheMiss
	}
	return string(entry.value), nil
}

func (c *MemoryCache) Set(_ context.Context, key string, value []byte, ttl time.Duration) error {
	// Copy the slice: callers may reuse the underlying buffer.
	cp := make([]byte, len(value))
	copy(cp, value)

	var exp time.Time
	if ttl > 0 {
		exp = time.Now().Add(ttl)
	}

	c.mu.Lock()
	c.items[key] = memoryEntry{value: cp, expiresAt: exp}
	c.mu.Unlock()
	return nil
}

func (c *MemoryCache) Ping(context.Context) error { return nil }

func (c *MemoryCache) Close() error {
	c.once.Do(func() { close(c.stopCh) })
	return nil
}

func (c *MemoryCache) janitor() {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			now := time.Now()
			c.mu.Lock()
			for k, v := range c.items {
				if !v.expiresAt.IsZero() && now.After(v.expiresAt) {
					delete(c.items, k)
				}
			}
			c.mu.Unlock()
		case <-c.stopCh:
			return
		}
	}
}
