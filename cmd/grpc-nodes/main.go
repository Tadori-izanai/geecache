package main

import (
	"flag"
	"fmt"
	"geecache/internal/geecache"
	peer "geecache/internal/geecache/grpc"
	"google.golang.org/grpc"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"

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
func startCacheServer(addr string, addrs []string, gee *geecache.Group) *grpc.Server {
	srv := peer.NewRPCPicker(addr)
	srv.Set(addrs...)
	gee.RegisterPicker(srv)

	log.Println("geecache is running at", addr)
	return srv.Server.StartServer()
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
	flag.IntVar(&port, "port", 8001, "Geecache server port")
	flag.BoolVar(&api, "api", false, "Whether start a api server")
	flag.Parse()

	apiAddr := "http://localhost:9999"
	addrMap := map[int]string{
		8001: "localhost:8001",
		8002: "localhost:8002",
		8003: "localhost:8003",
	}

	addrs := make([]string, 0, len(addrMap))
	for _, v := range addrMap {
		addrs = append(addrs, v)
	}

	gee := createGroup()
	if api {
		go startAPIServer(apiAddr, gee)
	}
	rpcSrv := startCacheServer(addrMap[port], addrs, gee)

	// signal
	c := make(chan os.Signal, 1)
	signal.Notify(c, syscall.SIGHUP, syscall.SIGQUIT, syscall.SIGTERM, syscall.SIGINT)
	for {
		s := <-c
		glog.Infof("geecache get a signal %s", s.String())
		switch s {
		case syscall.SIGQUIT, syscall.SIGTERM, syscall.SIGINT:
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
