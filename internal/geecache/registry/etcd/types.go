package etcd

import (
	"encoding/json"
	"path"
	"sort"
)

type Node struct {
	ID   string `json:"id"`   // unique, e.g. "localhost:8001"
	Addr string `json:"addr"` // e.g. "localhost:8001"
}

func nodeKey(prefix string, node Node) string {
	return path.Join(prefix, node.ID)
}

func encodeNode(node Node) (string, error) {
	data, err := json.Marshal(node)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func decodeNode(data []byte) (Node, error) {
	var node Node
	if err := json.Unmarshal(data, &node); err != nil {
		return Node{}, err
	}
	return node, nil
}

func mapToNodes(nodes map[string]Node) []Node {
	out := make([]Node, 0, len(nodes))
	for _, node := range nodes {
		out = append(out, node)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].Addr < out[j].Addr
	})
	return out
}
