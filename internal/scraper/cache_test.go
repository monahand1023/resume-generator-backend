package scraper

import (
	"fmt"
	"sync"
	"testing"
)

// TestLRUCache_setAndGet verifies that a value stored with set can be retrieved
// with get.
func TestLRUCache_setAndGet(t *testing.T) {
	c := newLRUCache(10)
	c.set("https://example.com/job/1", "job description text")

	got, ok := c.get("https://example.com/job/1")
	if !ok {
		t.Fatal("expected cache hit")
	}
	if got != "job description text" {
		t.Errorf("got %q, want %q", got, "job description text")
	}
}

// TestLRUCache_miss verifies that a get for an absent key returns ("", false).
func TestLRUCache_miss(t *testing.T) {
	c := newLRUCache(10)
	_, ok := c.get("https://example.com/not-stored")
	if ok {
		t.Error("expected cache miss")
	}
}

// TestLRUCache_update verifies that setting the same key twice keeps the most
// recent value.
func TestLRUCache_update(t *testing.T) {
	c := newLRUCache(10)
	c.set("k", "v1")
	c.set("k", "v2")

	got, ok := c.get("k")
	if !ok {
		t.Fatal("expected cache hit")
	}
	if got != "v2" {
		t.Errorf("got %q, want %q", got, "v2")
	}
}

// TestLRUCache_capacityEviction verifies that the oldest (least-recently-used)
// entry is evicted when the cache reaches capacity.
func TestLRUCache_capacityEviction(t *testing.T) {
	c := newLRUCache(3)

	c.set("a", "1")
	c.set("b", "2")
	c.set("c", "3")

	// "a" is now LRU; inserting "d" should evict it.
	c.set("d", "4")

	if _, ok := c.get("a"); ok {
		t.Error("expected 'a' to be evicted")
	}
	if _, ok := c.get("b"); !ok {
		t.Error("expected 'b' to still be present")
	}
	if _, ok := c.get("c"); !ok {
		t.Error("expected 'c' to still be present")
	}
	if _, ok := c.get("d"); !ok {
		t.Error("expected 'd' to be present")
	}
}

// TestLRUCache_getPromotesEntry verifies that a get makes the entry the MRU so
// it is not the next candidate for eviction.
func TestLRUCache_getPromotesEntry(t *testing.T) {
	c := newLRUCache(3)

	c.set("a", "1")
	c.set("b", "2")
	c.set("c", "3")

	// Access "a" to make it MRU; now "b" is LRU.
	c.get("a")

	// Adding "d" should evict "b", not "a".
	c.set("d", "4")

	if _, ok := c.get("a"); !ok {
		t.Error("expected 'a' to survive (was promoted by recent get)")
	}
	if _, ok := c.get("b"); ok {
		t.Error("expected 'b' to be evicted (was LRU after 'a' was accessed)")
	}
}

// TestLRUCache_singleCapacity verifies correct behaviour with capacity == 1.
func TestLRUCache_singleCapacity(t *testing.T) {
	c := newLRUCache(1)
	c.set("first", "v1")
	c.set("second", "v2")

	if _, ok := c.get("first"); ok {
		t.Error("expected 'first' to be evicted")
	}
	if got, ok := c.get("second"); !ok || got != "v2" {
		t.Errorf("expected 'second' with value 'v2', got %q ok=%v", got, ok)
	}
}

// TestLRUCache_concurrentAccess runs concurrent gets and sets to detect data
// races.  Run with `go test -race`.
func TestLRUCache_concurrentAccess(t *testing.T) {
	c := newLRUCache(50)
	const goroutines = 20
	const iterations = 100

	var wg sync.WaitGroup
	wg.Add(goroutines * 2)

	// Writers
	for g := 0; g < goroutines; g++ {
		go func(id int) {
			defer wg.Done()
			for i := 0; i < iterations; i++ {
				key := fmt.Sprintf("key-%d-%d", id, i%10)
				c.set(key, fmt.Sprintf("value-%d", i))
			}
		}(g)
	}

	// Readers
	for g := 0; g < goroutines; g++ {
		go func(id int) {
			defer wg.Done()
			for i := 0; i < iterations; i++ {
				key := fmt.Sprintf("key-%d-%d", id, i%10)
				c.get(key) //nolint:errcheck — we only care about no data race
			}
		}(g)
	}

	wg.Wait()
}

// TestLRUCache_panicOnZeroCapacity verifies that newLRUCache panics when given
// a non-positive capacity.
func TestLRUCache_panicOnZeroCapacity(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic for capacity 0")
		}
	}()
	newLRUCache(0)
}
