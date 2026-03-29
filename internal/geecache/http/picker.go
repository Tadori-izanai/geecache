package http

import (
	"geecache/internal/geecache"
	"geecache/pkg/consistenthash"
	"sync"
)

const defaultReplicas = 50

// HTTPPicker implements PeerPicker for a pool of HTTP peers.
type HTTPPicker struct {
	Pool    *HTTPPool
	peers   *consistenthash.Map
	getters map[string]geecache.PeerGetter // keyed by e.g. "http://10.0.0.2:8008"
	mu      sync.Mutex                     // guards peers and getters
}

var _ geecache.PeerPicker = new(HTTPPicker)

func NewHTTPPicker(addr string) *HTTPPicker {
	return &HTTPPicker{
		Pool:  NewHTTPPool(addr),
		peers: consistenthash.New(defaultReplicas, nil),
	}
}

// Set updates the pool's list of peers.
func (p *HTTPPicker) Set(peers ...string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.peers.Add(peers...)
	p.getters = make(map[string]geecache.PeerGetter, len(peers))
	for _, peer := range peers {
		p.getters[peer] = &httpGetter{baseURL: peer + p.Pool.basepath}
	}
}

// PickPeer picks a peer according to key
func (p *HTTPPicker) PickPeer(key string) (geecache.PeerGetter, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()

	// Important: the peer picked cannot be `self`
	if peer := p.peers.Get(key); peer != "" && peer != p.Pool.self {
		p.Pool.Log("Pick peer %s", peer)
		return p.getters[peer], true
	}
	return nil, false
}
