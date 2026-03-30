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
	//grpcBackoffMaxDelay       = time.Second * 3
)

type rpcGetter struct {
	addr   string
	client pb.GroupCacheClient
}

func newPRCGetter(addr string) *rpcGetter {
	return &rpcGetter{
		addr:   addr,
		client: newClient(addr),
	}
}

func newClient(addr string) pb.GroupCacheClient {
	conn, err := grpc.NewClient(addr,
		grpc.WithInsecure(),
		grpc.WithKeepaliveParams(keepalive.ClientParameters{
			Time:                grpcInitialWindowSize,
			Timeout:             grpcInitialConnWindowSize,
			PermitWithoutStream: true,
		}),
		grpc.WithInitialWindowSize(grpcInitialWindowSize),
		grpc.WithInitialConnWindowSize(grpcInitialWindowSize),
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
	//defer conn.Close()
	return pb.NewGroupCacheClient(conn)
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
