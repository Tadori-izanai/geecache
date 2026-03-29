package main

import (
	"flag"
	"fmt"
	"geecache/internal/geecache"
	peer "geecache/internal/geecache/http"
	"log"
	"net/http"
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
func startCacheServer(addr string, addrs []string, gee *geecache.Group) {
	srv := peer.NewHTTPPicker(addr)
	srv.Set(addrs...)
	gee.RegisterPicker(srv)

	log.Println("geecache is running at", addr)
	log.Fatal(http.ListenAndServe(addr[7:], srv.Pool))
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
		8001: "http://localhost:8001",
		8002: "http://localhost:8002",
		8003: "http://localhost:8003",
	}

	addrs := make([]string, 0, len(addrMap))
	for _, v := range addrMap {
		addrs = append(addrs, v)
	}

	gee := createGroup()
	if api {
		go startAPIServer(apiAddr, gee)
	}
	startCacheServer(addrMap[port], addrs, gee)
}
