package geecache

import (
	"fmt"
	"log"
	"net/http"
	"strings"
)

const defaultBasepath = "/_geecache/"

type HTTPPool struct {
	self     string // one's own address, including the hostname/IP and port
	basepath string // prefix for communication addresses between nodes
}

var _ http.Handler = new(HTTPPool)

func NewHTTPPool(self string) *HTTPPool {
	return &HTTPPool{
		self:     self,
		basepath: defaultBasepath,
	}
}

func (p *HTTPPool) Log(format string, args ...interface{}) {
	log.Printf("[Server %s] %s", p.self, fmt.Sprintf(format, args...))
}

func (p *HTTPPool) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if !strings.HasPrefix(r.URL.Path, p.basepath) {
		http.Error(w, "HTTPPool serving unexpected path: "+r.URL.Path, http.StatusNotFound)
		return
	}

	p.Log("%s %s", r.Method, r.URL.Path)

	// "/<basepath>/<groupname>/<key>" required
	parts := strings.SplitN(r.URL.Path[len(p.basepath):], "/", 2)
	if len(parts) != 2 {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	groupName, key := parts[0], parts[1]

	group := GetGroup(groupName)
	if group == nil {
		http.Error(w, "no such group: "+groupName, http.StatusNotFound)
		return
	}

	view, err := group.Get(key)
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
}
