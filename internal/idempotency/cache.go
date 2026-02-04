package idempotency

import (
	"container/list"
	"sync"
	"time"

	"github.com/codex-k8s/yaml-mcp-server/internal/protocol"
)

// Cache stores idempotent responses for a limited time.
type Cache struct {
	mu         sync.Mutex
	items      map[string]*list.Element
	order      *list.List
	ttl        time.Duration
	maxEntries int
	now        func() time.Time
}

type cacheEntry struct {
	key       string
	value     protocol.ToolResponse
	expiresAt time.Time
}

// NewCache creates a cache with the given ttl and max entries.
func NewCache(ttl time.Duration, maxEntries int) *Cache {
	if ttl <= 0 {
		ttl = time.Hour
	}
	if maxEntries <= 0 {
		maxEntries = 1000
	}
	return &Cache{
		items:      make(map[string]*list.Element),
		order:      list.New(),
		ttl:        ttl,
		maxEntries: maxEntries,
		now:        time.Now,
	}
}

// Get retrieves a cached response if present and not expired.
func (c *Cache) Get(key string) (protocol.ToolResponse, bool) {
	if c == nil || key == "" {
		return protocol.ToolResponse{}, false
	}
	c.mu.Lock()
	defer c.mu.Unlock()

	elem, ok := c.items[key]
	if !ok {
		return protocol.ToolResponse{}, false
	}
	entry := elem.Value.(*cacheEntry)
	if c.now().After(entry.expiresAt) {
		c.order.Remove(elem)
		delete(c.items, key)
		return protocol.ToolResponse{}, false
	}
	c.order.MoveToFront(elem)
	return entry.value, true
}

// Set stores a cached response.
func (c *Cache) Set(key string, value protocol.ToolResponse) {
	if c == nil || key == "" {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()

	if elem, ok := c.items[key]; ok {
		entry := elem.Value.(*cacheEntry)
		entry.value = value
		entry.expiresAt = c.now().Add(c.ttl)
		c.order.MoveToFront(elem)
		return
	}

	entry := &cacheEntry{
		key:       key,
		value:     value,
		expiresAt: c.now().Add(c.ttl),
	}
	elem := c.order.PushFront(entry)
	c.items[key] = elem
	c.trim()
}

func (c *Cache) trim() {
	for len(c.items) > c.maxEntries {
		elem := c.order.Back()
		if elem == nil {
			return
		}
		entry := elem.Value.(*cacheEntry)
		delete(c.items, entry.key)
		c.order.Remove(elem)
	}
}
