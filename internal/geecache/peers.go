package geecache

// PeerPicker is the interface that must be implemented to locate
// the peer that owns a specific key.
type PeerPicker interface {
	// PickPeer selects corresponding PeerGetter according to key.
	PickPeer(key string) (PeerGetter, bool)
}

// PeerGetter is the interface that must be implemented by a peer.
type PeerGetter interface {
	// Get gets $ value from the corresponding group
	Get(group string, key string) ([]byte, error)
}
