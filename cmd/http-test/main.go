package main

import (
	"fmt"
	"geecache/internal/geecache"
	http2 "geecache/internal/geecache/http"
	"log"
	"net/http"
)

var db = map[string]string{
	"Tom":  "630",
	"Jack": "589",
	"Sam":  "567",
}

func main() {
	callback := func(key string) ([]byte, error) {
		log.Println("[SlowDB] search key", key)
		if v, ok := db[key]; ok {
			return []byte(v), nil
		}
		return nil, fmt.Errorf("%s not exist", key)
	}

	geecache.NewGroup("scores", 2<<10, geecache.GetterFunc(callback))

	addr := "localhost:9999"
	peers := http2.NewHTTPPool(addr)

	log.Println("geecache is running at", addr)
	log.Fatal(http.ListenAndServe(addr, peers))
}
