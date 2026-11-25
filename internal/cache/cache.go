// Package cache is the LRU cache mpresty keeps remote graphics sources in.
//
//line cache.gw:29
//line cache.gw:30
package cache

import (
	"container/list"
	"sync"
)

// DefaultCapacity is the number of entries kept before eviction.
//
//line cache.gw:52
//line cache.gw:53
const DefaultCapacity = 4096

type entry struct {
	key string
	val string
}

// Cache is a safe-for-concurrent-use LRU of strings.
//
//line cache.gw:65
//line cache.gw:66
type Cache struct {
	mu    sync.Mutex
	cap   int
	ll    *list.List
	items map[string]*list.Element
}

// New returns a cache holding at most capacity entries. A capacity of zero
// or less means DefaultCapacity.
//
//line cache.gw:77
//line cache.gw:78
//line cache.gw:79
func New(capacity int) *Cache {
	if capacity <= 0 {
		capacity = DefaultCapacity
	}
	return &Cache{
		cap:   capacity,
		ll:    list.New(),
		items: make(map[string]*list.Element, capacity),
	}
}

// Get returns the value stored under key, if any.
//
//line cache.gw:94
//line cache.gw:95
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
//
//line cache.gw:113
//line cache.gw:114
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
//
//line cache.gw:134
func (c *Cache) Len() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.ll.Len()
}
