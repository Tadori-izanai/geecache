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

var _ geecache.PeerPicker = new(RPCPicker)

func NewRPCPicker(addr string) *RPCPicker {
	return &RPCPicker{
		Server: NewRPCServer(addr),
		peers:  consistenthash.New(defaultReplicas, nil),
	}
}

func (r *RPCPicker) Set(peers ...string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.peers.Add(peers...)
	r.getters = make(map[string]geecache.PeerGetter, len(peers))
	for _, peer := range peers {
		r.getters[peer] = newPRCGetter(peer)
	}
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
