package scraper

import (
	"container/list"
	"sync"
)

// lruCache is a simple, goroutine-safe LRU cache keyed by string with string
// values.  It uses a doubly-linked list (container/list) plus a hash map to
// achieve O(1) get and set operations.  No external dependencies are required.
type lruCache struct {
	capacity int
	mu       sync.Mutex
	list     *list.List
	items    map[string]*list.Element
}

// entry is the value stored inside each list.Element.
type entry struct {
	key   string
	value string
}

// newLRUCache returns an initialized lruCache with the given capacity.
// Panics if capacity <= 0.
func newLRUCache(capacity int) *lruCache {
	if capacity <= 0 {
		panic("lruCache: capacity must be > 0")
	}
	return &lruCache{
		capacity: capacity,
		list:     list.New(),
		items:    make(map[string]*list.Element, capacity),
	}
}

// get returns the cached value for key and true, or ("", false) on a miss.
// A hit moves the element to the front (most-recently-used position).
func (c *lruCache) get(key string) (string, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	el, ok := c.items[key]
	if !ok {
		return "", false
	}
	c.list.MoveToFront(el)
	return el.Value.(*entry).value, true
}

// set inserts or updates key with value.  If the cache is at capacity the
// least-recently-used element is evicted first.
func (c *lruCache) set(key string, value string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Update existing entry.
	if el, ok := c.items[key]; ok {
		c.list.MoveToFront(el)
		el.Value.(*entry).value = value
		return
	}

	// Evict LRU entry if at capacity.
	if c.list.Len() >= c.capacity {
		oldest := c.list.Back()
		if oldest != nil {
			c.list.Remove(oldest)
			delete(c.items, oldest.Value.(*entry).key)
		}
	}

	el := c.list.PushFront(&entry{key: key, value: value})
	c.items[key] = el
}
