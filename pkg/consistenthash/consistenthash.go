package consistenthash

import (
	"hash/crc32"
	"sort"
	"strconv"
)

type HashFunc func([]byte) uint32

var defaultHashFunc = crc32.ChecksumIEEE

// Map contains all hashed keys
type Map struct {
	hash     HashFunc
	replicas int
	keys     []int          // virtual nodes, sorted
	hashMap  map[int]string // virtual nodes -> real nodes
}

func New(replicas int, fn HashFunc) *Map {
	if fn == nil {
		fn = defaultHashFunc
	}
	return &Map{
		replicas: replicas,
		hash:     fn,
		hashMap:  make(map[int]string),
	}
}

// Add adds some keys (name of real nodes) to the hash.
func (m *Map) Add(keys ...string) {
	for _, key := range keys {
		for i := 0; i < m.replicas; i += 1 {
			d := []byte(strconv.Itoa(i) + key)
			hash := int(m.hash(d))
			m.keys = append(m.keys, hash)
			m.hashMap[hash] = key
		}
	}
	sort.Ints(m.keys)
}

// Remove removes a key (name of real a real node) and its virtual keys.
func (m *Map) Remove(keys string) {
	hashesToRemove := make(map[int]struct{})
	for i := 0; i < m.replicas; i += 1 {
		d := []byte(strconv.Itoa(i) + keys)
		hash := int(m.hash(d))
		hashesToRemove[hash] = struct{}{}
		delete(m.hashMap, hash)
	}

	newKeys := make([]int, 0, len(m.keys)-len(hashesToRemove))
	for _, key := range m.keys {
		if _, ok := hashesToRemove[key]; ok {
			continue
		}
		newKeys = append(newKeys, key)
	}
	m.keys = newKeys
}

// Get gets the closest item in the hash to the provided key.
func (m *Map) Get(key string) string {
	if len(m.keys) == 0 {
		return ""
	}

	hash := int(m.hash([]byte(key)))
	idx := sort.Search(len(m.keys), func(i int) bool {
		return m.keys[i] >= hash
	})
	idx %= len(m.keys) // if idx == len(m.keys) then use m.keys[0]

	return m.hashMap[m.keys[idx]]
}
