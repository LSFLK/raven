package cache

import (
	"sync"
	"time"
)

// Entry represents a cached item
type Entry struct {
	Exists     bool
	Data       string // For storing additional data (e.g., alias destination)
	Expires    time.Time
	LastUpdate time.Time
}

// Cache provides thread-safe caching with TTL
type Cache struct {
	store map[string]Entry
	mutex sync.RWMutex
	ttl   time.Duration
}

// New creates a new Cache instance
func New(ttlSeconds int) *Cache {
	return &Cache{
		store: make(map[string]Entry),
		ttl:   time.Duration(ttlSeconds) * time.Second,
	}
}

// Get retrieves an entry from cache
func (c *Cache) Get(key string) (Entry, bool) {
	c.mutex.RLock()
	defer c.mutex.RUnlock()
	entry, found := c.store[key]
	return entry, found
}

// Set stores an entry in cache
func (c *Cache) Set(key string, entry Entry) {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	c.store[key] = entry
}

// IsExpired checks if an entry has expired
func (c *Cache) IsExpired(entry Entry) bool {
	return time.Now().After(entry.Expires)
}

// GetTTL returns the cache TTL duration
func (c *Cache) GetTTL() time.Duration {
	return c.ttl
}
