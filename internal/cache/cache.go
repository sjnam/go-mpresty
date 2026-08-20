// Package cache replaces mpresty's `lua_shared_dict mpresty_cache 10m`. A
// shared dict was per-worker and byte-bounded; a single Go process needs
// neither, so this is a plain entry-bounded LRU guarded by a mutex.
package cache

import (
	"container/list"
	"sync"
)

// DefaultCapacity is the number of entries kept before eviction.
const DefaultCapacity = 4096

type entry struct {
	key string
	val string
}

// Cache is a safe-for-concurrent-use LRU of strings.
type Cache struct {
	mu    sync.Mutex
	cap   int
	ll    *list.List
	items map[string]*list.Element
}

// New returns a cache holding at most capacity entries. A capacity of zero or
// less means DefaultCapacity.
func New(capacity int) *Cache {
	if capacity <= 0 {
		capacity = DefaultCapacity
	}
	return &Cache{cap: capacity, ll: list.New(), items: make(map[string]*list.Element, capacity)}
}

// Get returns the value stored under key, if any.
func (c *Cache) Get(key string) (string, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	el, ok := c.items[key]
	if !ok {
		return "", false
	}
	c.ll.MoveToFront(el)
	return el.Value.(*entry).val, true
}

// Set stores a value, evicting the least recently used entry if full.
func (c *Cache) Set(key, val string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if el, ok := c.items[key]; ok {
		c.ll.MoveToFront(el)
		el.Value.(*entry).val = val
		return
	}
	c.items[key] = c.ll.PushFront(&entry{key: key, val: val})
	for c.ll.Len() > c.cap {
		if el := c.ll.Back(); el != nil {
			c.ll.Remove(el)
			delete(c.items, el.Value.(*entry).key)
		}
	}
}

// Len reports how many entries are held.
func (c *Cache) Len() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.ll.Len()
}
