package query

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"sync"
	"time"

	lru "github.com/hashicorp/golang-lru/v2/expirable"
	"golang.org/x/sync/singleflight"
)

// ResultCache caches chart result sets keyed by a canonical hash of the query.
type ResultCache struct {
	cache   *lru.LRU[string, *ResultSet]
	group   singleflight.Group
	mu      sync.RWMutex
	enabled bool
}

func NewResultCache(ttl time.Duration, maxEntries int) *ResultCache {
	if ttl <= 0 {
		ttl = 60 * time.Second
	}
	if maxEntries <= 0 {
		maxEntries = 500
	}
	return &ResultCache{
		cache:   lru.NewLRU[string, *ResultSet](maxEntries, nil, ttl),
		enabled: true,
	}
}

// CacheKey builds a stable hash from dataset/chart versions, canonical params, mode and limit.
func CacheKey(parts ...any) string {
	h := sha256.New()
	for _, p := range parts {
		if s, ok := p.(string); ok {
			h.Write([]byte(s))
		} else {
			b, _ := json.Marshal(p)
			h.Write(b)
		}
		h.Write([]byte{0})
	}
	return hex.EncodeToString(h.Sum(nil))
}

// GetOrLoad returns a cached result or executes load once per key.
func (c *ResultCache) GetOrLoad(key string, load func() (*ResultSet, error)) (*ResultSet, bool, error) {
	if c == nil || !c.enabled {
		rs, err := load()
		return rs, false, err
	}
	c.mu.RLock()
	cacheRef := c.cache
	c.mu.RUnlock()
	if v, ok := cacheRef.Get(key); ok {
		return v, true, nil
	}
	res, err, _ := c.group.Do(key, func() (any, error) {
		rs, err := load()
		if err != nil {
			return nil, err
		}
		// Only cache modest payloads.
		if len(rs.Rows) <= 5000 {
			c.mu.RLock()
			cacheRef := c.cache
			c.mu.RUnlock()
			cacheRef.Add(key, rs)
		}
		return rs, nil
	})
	if err != nil {
		return nil, false, err
	}
	return res.(*ResultSet), false, nil
}

// Reconfigure swaps in new TTL/size limits, discarding existing entries because
// the LRU size is fixed at construction.
func (c *ResultCache) Reconfigure(ttl time.Duration, maxEntries int) {
	if c == nil {
		return
	}
	if ttl <= 0 {
		ttl = 60 * time.Second
	}
	if maxEntries <= 0 {
		maxEntries = 500
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.cache = lru.NewLRU[string, *ResultSet](maxEntries, nil, ttl)
	c.enabled = true
}

// Enabled reports whether results are cached.
func (c *ResultCache) Enabled() bool {
	if c == nil {
		return false
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.enabled
}

func (c *ResultCache) Clear() {
	if c == nil {
		return
	}
	c.mu.RLock()
	cacheRef := c.cache
	c.mu.RUnlock()
	cacheRef.Purge()
}
