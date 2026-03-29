package geecache

import (
	"geecache/pkg/lru"
)

// ByteView is a readonly data structure, used to represent $ values
type ByteView struct {
	b []byte
}

var _ lru.Value = ByteView{}

func (bv ByteView) Len() int {
	return len(bv.b)
}

func (bv ByteView) ByteSlice() []byte {
	return cloneBytes(bv.b)
}

func cloneBytes(b []byte) []byte {
	ret := make([]byte, len(b))
	copy(ret, b)
	return ret
}

func (bv ByteView) String() string {
	return string(bv.b)
}
