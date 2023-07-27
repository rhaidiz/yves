package main

import (
	"fmt"
	"net/http"
	"net/url"

	"github.com/rhaidiz/yves"
)

func main() {
	endProxy := yves.NewProxy()

	startProxy := yves.NewProxy()

	go http.ListenAndServe("127.0.0.1:8081", endProxy)

	endProxy.HandleRequest = func(i int64, r *http.Request) *http.Response {
		fmt.Println("traversing end proxy")
		return nil
	}

	startProxy.HandleRequest = func(i int64, r *http.Request) *http.Response {
		fmt.Println("traversing start proxy")
		return nil
	}

	proxyUrl, _ := url.Parse("http://127.0.0.1:8081")
	startProxy.Tr.Proxy = http.ProxyURL(proxyUrl)

	http.ListenAndServe("127.0.0.1:8080", startProxy)
}
