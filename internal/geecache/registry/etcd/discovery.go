package etcd

import (
	"context"
	"errors"

	"go.etcd.io/etcd/api/v3/mvccpb"
	"go.etcd.io/etcd/api/v3/v3rpc/rpctypes"
	clientv3 "go.etcd.io/etcd/client/v3"
)

type Discovery struct {
	cli    *clientv3.Client
	prefix string
}

func NewDiscovery(cli *clientv3.Client, prefix string) *Discovery {
	return &Discovery{cli: cli, prefix: prefix}
}

// List returns the current snapshot only.
// It is fine for inspection, but it is not enough to build a race-free
// long-running discovery loop because updates can happen right after it returns.
func (d *Discovery) List(ctx context.Context) ([]Node, error) {
	nodes, _, err := d.snapshot(ctx)
	if err != nil {
		return nil, err
	}
	return mapToNodes(nodes), nil
}

// SyncAndWatch first loads a snapshot and its revision, then watches from the
// next revision so no membership change between the two steps is missed.
//
// The important etcd property here is:
// - Get returns a point-in-time snapshot plus its revision R
// - Watch started with WithRev(R+1) will deliver every change after that snapshot
//
// This is why we do not simply "open watch, then list":
//   - creating a local watch channel does not mean the watch is already established
//     on the etcd server
//   - even if both succeed, without a shared revision there is no precise boundary
//     between "already included in snapshot" and "must come from watch"
func (d *Discovery) SyncAndWatch(ctx context.Context, onUpdate func([]Node)) error {
	for {
		nodes, rev, err := d.snapshot(ctx)
		if err != nil {
			return err
		}
		onUpdate(mapToNodes(nodes))

		err = d.watchFromRevision(ctx, rev+1, nodes, onUpdate)
		if err == nil || errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return err
		}
		if errors.Is(err, rpctypes.ErrCompacted) {
			continue
		}
		return err
	}
}

func (d *Discovery) watchFromRevision(ctx context.Context, rev int64, nodes map[string]Node, onUpdate func([]Node)) error {
	watchCh := d.cli.Watch(ctx, d.prefix, clientv3.WithPrefix(), clientv3.WithRev(rev))
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

func (d *Discovery) snapshot(ctx context.Context) (map[string]Node, int64, error) {
	resp, err := d.cli.Get(ctx, d.prefix, clientv3.WithPrefix())
	if err != nil {
		return nil, 0, err
	}

	nodes := make(map[string]Node, len(resp.Kvs))
	for _, kv := range resp.Kvs {
		node, err := decodeNode(kv.Value)
		if err != nil {
			return nil, 0, err
		}
		nodes[string(kv.Key)] = node
	}
	return nodes, resp.Header.Revision, nil
}
