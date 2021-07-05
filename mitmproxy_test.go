package yves

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
)

func TestMitmProxy(t *testing.T) {
	const backendResponse = "I am the backend"
	const backendStatus = 404

	// a backend example
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, backendResponse)
		return
	}))

	// create yves
	proxy := GetDefault()

	go http.ListenAndServe(":8080", proxy)

	proxyUrl, err := url.Parse("http://127.0.0.1:8080")
	myClient := &http.Client{Transport: &http.Transport{Proxy: http.ProxyURL(proxyUrl)}}

	_, err = myClient.Get(backend.URL)
	if err != nil {
		t.Errorf("There was an error")
	}

}
