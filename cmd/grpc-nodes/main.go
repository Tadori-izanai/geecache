package main

import (
	"context"
	"flag"
	"fmt"
	"geecache/internal/geecache"
	peer "geecache/internal/geecache/grpc"
	registry "geecache/internal/geecache/registry/etcd"
	"google.golang.org/grpc"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/golang/glog"
)

var db = map[string]string{
	"Tom":  "630",
	"Jack": "589",
	"Sam":  "567",
}

// createGroup creates a namespace
func createGroup() *geecache.Group {
	callback := func(key string) ([]byte, error) {
		log.Println("[SlowDB] search key", key)
		if v, ok := db[key]; ok {
			return []byte(v), nil
		}
		return nil, fmt.Errorf("%s not exist", key)
	}

	return geecache.NewGroup("scores", 2<<10, geecache.GetterFunc(callback))
}

// startCacheServer sets local node `addr` and remote nodes `addrs` for given namespace
func startCacheServer(addr string, gee *geecache.Group) (*peer.RPCPicker, *grpc.Server) {
	srv := peer.NewRPCPicker(addr)
	gee.RegisterPicker(srv)

	log.Println("geecache is running at", addr)
	return srv, srv.Server.StartServer()
}

// startAPIServer sets an API service (port 9999) and interacts with users, for given namespace
func startAPIServer(apiAddr string, gee *geecache.Group) {
	http.HandleFunc("/api", func(w http.ResponseWriter, r *http.Request) {
		key := r.URL.Query().Get("key")
		view, err := gee.Get(key)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/octet-stream")
		_, err = w.Write(view.ByteSlice())
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	})

	log.Println("fontend server is running at", apiAddr)
	log.Fatal(http.ListenAndServe(apiAddr[7:], nil))
}

func main() {
	var port int
	var api bool
	var etcdEndpoints string
	var servicePrefix string
	var nodeID string
	flag.IntVar(&port, "port", 8001, "Geecache server port")
	flag.BoolVar(&api, "api", false, "Whether start a api server")
	flag.StringVar(&etcdEndpoints, "etcd", "127.0.0.1:2379", "Comma-separated etcd endpoints")
	flag.StringVar(&servicePrefix, "service-prefix", "/services/geecache/nodes/", "Service prefix in etcd")
	flag.StringVar(&nodeID, "node-id", "", "Unique node ID in etcd, defaults to addr")
	flag.Parse()

	apiAddr := "http://localhost:9999"
	addrMap := map[int]string{
		8001: "localhost:8001",
		8002: "localhost:8002",
		8003: "localhost:8003",
	}

	addr, ok := addrMap[port]
	if !ok {
		log.Fatalf("unknown port: %d", port)
	}
	if nodeID == "" {
		nodeID = addr
	}

	gee := createGroup()
	if api {
		go startAPIServer(apiAddr, gee)
	}
	picker, rpcSrv := startCacheServer(addr, gee)

	cli, err := registry.NewClient(strings.Split(etcdEndpoints, ","))
	if err != nil {
		log.Fatal(err)
	}
	defer cli.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	registrar := registry.NewRegistrar(cli, servicePrefix, registry.Node{ID: nodeID, Addr: addr})
	if err := registrar.Register(ctx, 10); err != nil {
		log.Fatal(err)
	}

	discovery := registry.NewDiscovery(cli, servicePrefix)
	nodes, err := discovery.List(ctx)
	if err != nil {
		log.Fatal(err)
	}
	picker.UpdatePeers(nodeAddrs(nodes))

	go func() {
		err := discovery.Watch(ctx, func(nodes []registry.Node) {
			picker.UpdatePeers(nodeAddrs(nodes))
		})
		if err != nil && ctx.Err() == nil {
			log.Printf("etcd discovery stopped: %v", err)
		}
	}()

	c := make(chan os.Signal, 1)
	signal.Notify(c, syscall.SIGHUP, syscall.SIGQUIT, syscall.SIGTERM, syscall.SIGINT)
	for {
		s := <-c
		glog.Infof("geecache get a signal %s", s.String())
		switch s {
		case syscall.SIGQUIT, syscall.SIGTERM, syscall.SIGINT:
			cancel()
			shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 3*time.Second)
			if err := registrar.Deregister(shutdownCtx); err != nil {
				log.Printf("failed to deregister node: %v", err)
			}
			shutdownCancel()
			rpcSrv.GracefulStop()
			glog.Infof("geecache exit")
			glog.Flush()
			return
		case syscall.SIGHUP:
		default:
			return
		}
	}
}

func nodeAddrs(nodes []registry.Node) []string {
	addrs := make([]string, 0, len(nodes))
	for _, node := range nodes {
		if node.Addr == "" {
			continue
		}
		addrs = append(addrs, node.Addr)
	}
	return addrs
}
