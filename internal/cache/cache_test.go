//line cache.gw:145
package cache

import (
	"strconv"
	"sync"
	"testing"
)

func TestEvictsLeastRecentlyUsed(t *testing.T) {
	c := New(2)
	c.Set("a", "1")
	c.Set("b", "2")
	if _, ok := c.Get("a"); !ok {
		t.Fatal("a missing")
	}
	c.Set("c", "3")
	if _, ok := c.Get("b"); ok {
		t.Error("b should have been evicted")
	}
	if v, ok := c.Get("a"); !ok || v != "1" {
		t.Errorf("a = %q, %v", v, ok)
	}
	if n := c.Len(); n != 2 {
		t.Errorf("Len = %d, want 2", n)
	}
}

//line cache.gw:177
func TestConcurrentUse(t *testing.T) {
	c := New(64)
	var wg sync.WaitGroup
	for i := range 32 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			k := strconv.Itoa(i)
			c.Set(k, k)
			c.Get(k)
		}()
	}
	wg.Wait()
}
