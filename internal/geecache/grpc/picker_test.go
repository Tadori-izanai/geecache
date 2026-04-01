package grpc

import "testing"

func TestRPCPickerUpdatePeersRemovesOfflineNodes(t *testing.T) {
	picker := NewRPCPicker("self")

	picker.UpdatePeers([]string{"self", "node-a", "node-b"})
	if _, ok := picker.getters["node-a"]; !ok {
		t.Fatalf("expected node-a getter to exist after initial update")
	}

	picker.UpdatePeers([]string{"self", "node-b"})

	if _, ok := picker.getters["node-a"]; ok {
		t.Fatalf("expected node-a getter to be removed after update")
	}
	if _, ok := picker.getters["node-b"]; !ok {
		t.Fatalf("expected node-b getter to remain after update")
	}
}

func TestRPCPickerNeverReturnsRemovedPeer(t *testing.T) {
	picker := NewRPCPicker("self")
	picker.UpdatePeers([]string{"self", "node-a", "node-b"})
	picker.UpdatePeers([]string{"self", "node-b"})

	for i := 0; i < 2048; i++ {
		key := testKey(i)
		getter, ok := picker.PickPeer(key)
		if !ok {
			continue
		}
		rg, ok := getter.(*rpcGetter)
		if !ok {
			t.Fatalf("expected rpcGetter, got %T", getter)
		}
		if rg.addr == "node-a" {
			t.Fatalf("key %q routed to removed peer node-a", key)
		}
	}
}

func testKey(i int) string {
	return "key-" + string(rune('a'+(i%26))) + "-" + string(rune('0'+(i%10)))
}
