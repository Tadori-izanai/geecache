package geecache

import (
	"errors"
	"geecache/internal/geecache/singleflight"
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
//
// For process (2):
//
// 使用一致性哈希选择节点        Y                                    Y
//     |-----> 是否是远程节点 -----> HTTP 客户端访问远程节点 --> 成功？-----> 服务端返回返回值
//                     |  N                                    ↓  N
//                     |----------------------------> 回退到本地节点处理。

// A Group is a $ namespace and associated data loaded spread over
type Group struct {
	name       string
	getter     Getter
	mainCache  cache
	peerPicker PeerPicker
	// use singleflight.Group to make sure that each key is only fetched once
	loader *singleflight.Group
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
		loader:    &singleflight.Group{},
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

// RegisterPicker registers a PeerPicker for choosing remote peer
func (g *Group) RegisterPicker(peer PeerPicker) {
	if g.peerPicker != nil {
		panic("RegisterPicker called more than once")
	}
	g.peerPicker = peer
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
	// process (2) and (3)
	bv, err := g.loader.Do(key, func() (any, error) {
		return g.load(key)
	})
	if err == nil {
		return bv.(ByteView), nil
	}
	return ByteView{}, err
}

func (g *Group) load(key string) (ByteView, error) {
	// process (2)
	if g.peerPicker != nil {
		if peer, ok := g.peerPicker.PickPeer(key); ok {
			if value, err := g.getFromPeer(peer, key); err == nil {
				return value, nil
			}
			log.Println("[GeeCache] Failed to get from peer", peer)
		}
	}

	// process (3)
	return g.getLocally(key)
}

func (g *Group) getFromPeer(peer PeerGetter, key string) (ByteView, error) {
	bytes, err := peer.Get(g.name, key)
	if err != nil {
		return ByteView{}, err
	}
	return ByteView{b: bytes}, nil
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
