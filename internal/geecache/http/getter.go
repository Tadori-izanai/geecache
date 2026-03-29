package http

import (
	"fmt"
	"geecache/internal/geecache"
	"io"
	"net/http"
	"net/url"
)

type httpGetter struct {
	// baseURL represents the address of the remote node to be accessed, and
	// ends with a slash, such as http://example.com/_geecache/
	baseURL string
}

var _ geecache.PeerGetter = new(httpGetter)

func (h *httpGetter) Get(group, key string) ([]byte, error) {
	// url.QueryEscape ensures that special characters in a query parameter value
	// are correctly transmitted without being misinterpreted as part of the URL structure itself.
	//    e.g. "Hello World! &" -> "Hello+World%21+%26"
	u := fmt.Sprintf("%v%v/%v", h.baseURL, url.QueryEscape(group), url.QueryEscape(key))

	resp, err := http.Get(u)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("server returned: %v", resp.Status)
	}

	bytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading response body: %v", err)
	}
	return bytes, nil
}
