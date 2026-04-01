package etcd

import (
	"context"
	"fmt"

	clientv3 "go.etcd.io/etcd/client/v3"
)

type Registrar struct {
	cli     *clientv3.Client
	prefix  string
	node    Node
	leaseID clientv3.LeaseID
}

func NewRegistrar(cli *clientv3.Client, prefix string, node Node) *Registrar {
	return &Registrar{
		cli:    cli,
		prefix: prefix,
		node:   node,
	}
}

func (r *Registrar) Register(ctx context.Context, ttl int64) error {
	if r.node.ID == "" {
		return fmt.Errorf("etcd registrar: node id is required")
	}
	if r.node.Addr == "" {
		return fmt.Errorf("etcd registrar: node addr is required")
	}

	leaseResp, err := r.cli.Grant(ctx, ttl)
	if err != nil {
		return err
	}

	value, err := encodeNode(r.node)
	if err != nil {
		return err
	}

	r.leaseID = leaseResp.ID
	_, err = r.cli.Put(ctx, nodeKey(r.prefix, r.node), value, clientv3.WithLease(r.leaseID))
	if err != nil {
		return err
	}

	keepAliveCh, err := r.cli.KeepAlive(ctx, r.leaseID)
	if err != nil {
		return err
	}

	go func() {
		for range keepAliveCh {
		}
	}()

	return nil
}

func (r *Registrar) Deregister(ctx context.Context) error {
	if r.leaseID == 0 {
		return nil
	}
	_, err := r.cli.Revoke(ctx, r.leaseID)
	if err != nil {
		return err
	}
	r.leaseID = 0
	return nil
}
