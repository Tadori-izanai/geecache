package etcd

import (
	"context"

	"go.etcd.io/etcd/api/v3/mvccpb"
	clientv3 "go.etcd.io/etcd/client/v3"
)

type Discovery struct {
	cli    *clientv3.Client
	prefix string
}

func NewDiscovery(cli *clientv3.Client, prefix string) *Discovery {
	return &Discovery{cli: cli, prefix: prefix}
}

func (d *Discovery) List(ctx context.Context) ([]Node, error) {
	nodes, err := d.listNodeMap(ctx)
	if err != nil {
		return nil, err
	}
	return mapToNodes(nodes), nil
}

func (d *Discovery) Watch(ctx context.Context, onUpdate func([]Node)) error {
	nodes, err := d.listNodeMap(ctx)
	if err != nil {
		return err
	}
	onUpdate(mapToNodes(nodes))

	watchCh := d.cli.Watch(ctx, d.prefix, clientv3.WithPrefix())
	for watchResp := range watchCh {
		if err := watchResp.Err(); err != nil {
			return err
		}
		for _, ev := range watchResp.Events {
			switch ev.Type {
			case mvccpb.PUT:
				node, err := decodeNode(ev.Kv.Value)
				if err != nil {
					return err
				}
				nodes[string(ev.Kv.Key)] = node
			case mvccpb.DELETE:
				delete(nodes, string(ev.Kv.Key))
			}
		}
		onUpdate(mapToNodes(nodes))
	}

	return ctx.Err()
}

func (d *Discovery) listNodeMap(ctx context.Context) (map[string]Node, error) {
	resp, err := d.cli.Get(ctx, d.prefix, clientv3.WithPrefix())
	if err != nil {
		return nil, err
	}

	nodes := make(map[string]Node, len(resp.Kvs))
	for _, kv := range resp.Kvs {
		node, err := decodeNode(kv.Value)
		if err != nil {
			return nil, err
		}
		nodes[string(kv.Key)] = node
	}
	return nodes, nil
}
