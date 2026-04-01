package grpc

import (
	"geecache/internal/geecache"
	"geecache/pkg/consistenthash"
	"sync"
)

const defaultReplicas = 50

type RPCPicker struct {
	Server  *RPCServer
	peers   *consistenthash.Map
	getters map[string]geecache.PeerGetter
	mu      sync.RWMutex
}

type getterCloser interface {
	Close() error
}

var _ geecache.PeerPicker = new(RPCPicker)

func NewRPCPicker(addr string) *RPCPicker {
	return &RPCPicker{
		Server:  NewRPCServer(addr),
		peers:   consistenthash.New(defaultReplicas, nil),
		getters: make(map[string]geecache.PeerGetter),
	}
}

func (r *RPCPicker) Set(peers ...string) {
	r.UpdatePeers(peers)
}

func (r *RPCPicker) UpdatePeers(peers []string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	nextHash := consistenthash.New(defaultReplicas, nil)
	nextHash.Add(peers...)

	nextGetters := make(map[string]geecache.PeerGetter, len(peers))
	for _, peer := range peers {
		if old, ok := r.getters[peer]; ok {
			nextGetters[peer] = old
			continue
		}
		nextGetters[peer] = newRPCGetter(peer)
	}

	for peer, getter := range r.getters {
		if _, ok := nextGetters[peer]; ok {
			continue
		}
		closer, ok := getter.(getterCloser)
		if ok {
			_ = closer.Close()
		}
	}

	r.peers = nextHash
	r.getters = nextGetters
}

func (r *RPCPicker) PickPeer(key string) (geecache.PeerGetter, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if peer := r.peers.Get(key); peer != "" && peer != r.Server.self {
		r.Server.Log("Pick peer %s", peer)
		return r.getters[peer], true
	}
	return nil, false
}
