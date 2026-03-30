package grpc

import (
	"context"
	"fmt"
	pb "geecache/api/geecache"
	"geecache/internal/geecache"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/keepalive"
	"google.golang.org/grpc/status"
	"log"
	"net"
	"time"
)

type RPCServer struct {
	self string // hostname/IP and port
}

func NewRPCServer(self string) *RPCServer {
	return &RPCServer{
		self: self,
	}
}

func (r *RPCServer) StartServer() *grpc.Server {
	keepParams := grpc.KeepaliveParams(keepalive.ServerParameters{
		MaxConnectionIdle:     time.Second * 60,
		MaxConnectionAgeGrace: time.Second * 20,
		Time:                  time.Second * 60,
		Timeout:               time.Second * 20,
		MaxConnectionAge:      time.Hour * 2,
	})
	srv := grpc.NewServer(keepParams)
	pb.RegisterGroupCacheServer(srv, &server{srv: r})

	lis, err := net.Listen("tcp", r.self)
	if err != nil {
		panic(err)
	}
	go func() {
		if err := srv.Serve(lis); err != nil {
			panic(err)
		}
	}()
	return srv
}

func (r *RPCServer) Log(format string, args ...interface{}) {
	log.Printf("[Server %s] %s", r.self, fmt.Sprintf(format, args...))
}

type server struct {
	pb.UnimplementedGroupCacheServer
	srv *RPCServer
}

var _ pb.GroupCacheServer = &server{}

func (s *server) Get(ctx context.Context, req *pb.Request) (*pb.Response, error) {
	s.srv.Log("Get %s/%s", req.Group, req.Key)

	group := geecache.GetGroup(req.Group)
	if group == nil {
		return nil, status.Errorf(codes.NotFound, "group not found: %s", req.Group)
	}

	view, err := group.Get(req.Key)
	if err != nil {
		return nil, status.Errorf(codes.NotFound, "key not found: %s", req.Key)
	}
	return &pb.Response{Value: view.ByteSlice()}, nil
}
