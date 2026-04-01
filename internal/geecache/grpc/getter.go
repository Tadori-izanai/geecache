package grpc

import (
	"context"
	pb "geecache/api/geecache"
	"geecache/internal/geecache"
	"google.golang.org/grpc"
	"google.golang.org/grpc/keepalive"
	"time"
)

const (
	// grpc options
	grpcInitialWindowSize     = 1 << 24
	grpcInitialConnWindowSize = 1 << 24
	grpcMaxSendMsgSize        = 1 << 24
	grpcMaxCallMsgSize        = 1 << 24
	grpcKeepAliveTime         = time.Second * 10
	grpcKeepAliveTimeout      = time.Second * 3
)

type rpcGetter struct {
	addr   string
	conn   *grpc.ClientConn
	client pb.GroupCacheClient
}

func newRPCGetter(addr string) *rpcGetter {
	conn, client := newClient(addr)
	return &rpcGetter{
		addr:   addr,
		conn:   conn,
		client: client,
	}
}

func newClient(addr string) (*grpc.ClientConn, pb.GroupCacheClient) {
	conn, err := grpc.NewClient(addr,
		grpc.WithInsecure(),
		grpc.WithInitialWindowSize(grpcInitialWindowSize),
		grpc.WithInitialConnWindowSize(grpcInitialConnWindowSize),
		grpc.WithDefaultCallOptions(grpc.MaxCallRecvMsgSize(grpcMaxCallMsgSize)),
		grpc.WithDefaultCallOptions(grpc.MaxCallSendMsgSize(grpcMaxSendMsgSize)),
		grpc.WithKeepaliveParams(keepalive.ClientParameters{
			Time:                grpcKeepAliveTime,
			Timeout:             grpcKeepAliveTimeout,
			PermitWithoutStream: true,
		}),
	)
	if err != nil {
		panic(err)
	}
	return conn, pb.NewGroupCacheClient(conn)
}

var _ geecache.PeerGetter = new(rpcGetter)

func (r *rpcGetter) Get(group, key string) ([]byte, error) {
	reply, err := r.client.Get(context.TODO(), &pb.Request{
		Group: group,
		Key:   key,
	})
	if err != nil {
		return nil, err
	}
	return reply.Value, nil
}

func (r *rpcGetter) Close() error {
	return r.conn.Close()
}
