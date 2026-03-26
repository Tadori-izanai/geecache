package lru

import "container/list"

// Value use Len to count how many bytes it takes
type Value interface {
	Len() int
}

// Cache is a LRU $. It is not safe for concurrent access.
type Cache struct {
	maxBytes int64      // the maximum allowed memory, not specified if maxBytes is 0.
	nbytes   int64      // currently used memory
	ll       *list.List // DLList, front is the most recently used
	cache    map[string]*list.Element

	// optional and executed when an entry is purged.
	OnEvicted func(key string, value Value)
}

// *entry is the datatype in nodes list.Element (.Value)
// The key corresponding to each value is still saved in the linked list
type entry struct {
	key   string
	value Value
}

func New(maxBytes int64, onEvicted func(string, Value)) *Cache {
	return &Cache{
		maxBytes:  maxBytes,
		ll:        list.New(),
		cache:     make(map[string]*list.Element),
		OnEvicted: onEvicted,
	}
}

func (c *Cache) Get(key string) (Value, bool) {
	elem, ok := c.cache[key]
	if ok {
		c.ll.MoveToFront(elem)
		kv := elem.Value.(*entry)
		return kv.value, true
	}
	return nil, false
}

func (c *Cache) RemoveOldest() {
	elem := c.ll.Back()
	if elem == nil {
		return
	}
	kv := c.ll.Remove(elem).(*entry)
	delete(c.cache, kv.key)
	c.nbytes -= int64(len(kv.key) + kv.value.Len())
	if c.OnEvicted != nil {
		c.OnEvicted(kv.key, kv.value)
	}
}

// Add does:
// if key exists, then update it,
// else push kv to front.
// if c.nbytes exceeds c.maxBytes, then remove oldest.
func (c *Cache) Add(key string, value Value) {
	if elem, ok := c.cache[key]; ok {
		c.ll.MoveToFront(elem)
		kv := elem.Value.(*entry)
		c.nbytes += int64(-kv.value.Len() + value.Len())
		kv.value = value
	} else {
		elem := c.ll.PushFront(&entry{key, value})
		c.cache[key] = elem
		c.nbytes += int64(len(key) + value.Len())
	}
	for c.maxBytes != 0 && c.nbytes > c.maxBytes {
		c.RemoveOldest()
	}
}

func (c *Cache) Len() int {
	return c.ll.Len()
}
