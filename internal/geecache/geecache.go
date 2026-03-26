package geecache

import (
	"errors"
	"log"
	"sync"
)

type Getter interface {
	Get(key string) ([]byte, error)
}

type GetterFunc func(key string) ([]byte, error)

// GetterFunc 是一个实现了接口的函数类型，简称为接口型函数
var _ Getter = GetterFunc(nil)

func (f GetterFunc) Get(key string) ([]byte, error) {
	return f(key)
}

//                              Y
// 接收 key --> 检查是否被缓存 -----> 返回缓存值 (1)
//                 |  N                           Y
//                 |-----> 是否应当从远程节点获取 -----> 与远程节点交互 --> 返回缓存值 (2)
//                     |  N
//                     |-----> 调用回调函数 `getter`，获取值并添加到缓存 --> 返回缓存值 (3)

// A Group implements (1) and (3)
// A Group is a $ namespace and associated data loaded spread over
type Group struct {
	name      string
	getter    Getter
	mainCache cache
}

var (
	mu     sync.RWMutex
	groups = make(map[string]*Group)
)

// NewGroup create a new instance of Group
func NewGroup(name string, cacheBytes int64, getter Getter) *Group {
	if getter == nil {
		panic("geecache: getter is nil")
	}
	g := &Group{
		name:      name,
		getter:    getter,
		mainCache: cache{cacheBytes: cacheBytes},
	}

	mu.Lock()
	defer mu.Unlock()
	groups[name] = g

	return g
}

func GetGroup(name string) *Group {
	mu.RLock()
	defer mu.RUnlock()
	return groups[name]
}

// Get value for a key from $
func (g *Group) Get(key string) (ByteView, error) {
	if key == "" {
		return ByteView{}, errors.New("geecache: key is required")
	}

	// process (1)
	if v, ok := g.mainCache.get(key); ok {
		log.Println("[GeeCache] hit")
		return v, nil
	}
	// process (3)
	return g.load(key)
}

func (g *Group) load(key string) (ByteView, error) {
	return g.getLocally(key)
}

func (g *Group) getLocally(key string) (ByteView, error) {
	b, err := g.getter.Get(key)
	if err != nil {
		return ByteView{}, err
	}
	value := ByteView{b: b}
	g.populateCache(key, value)
	return value, nil
}

func (g *Group) populateCache(key string, value ByteView) {
	g.mainCache.add(key, value)
}
