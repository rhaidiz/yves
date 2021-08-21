# Instroduction
Yves is what eveyone was wating for, an HTTP(s) intercept proxy.
Yes, that's pretty much it.

# Features
* HTTP\S man-in-the-middle proxy;
* Custom HTTP request handlers;
* Custom HTTP responses handlers;

# Usage

## Start a server
The following snippets of code shows how to start a simple mitm proxy.
More usage examples can be found in the examples folder.

```go
package main

import (
        "log"
        "net/http"

        "github.com/rhaidiz/yves"
)

func main() {

        // create a new mitm proxy
        proxy := &yves.Proxy{}

        // Listen on local port 8080
        log.Fatal(http.ListenAndServe(":8080", proxy))
}
```

## Request handler
The following example shows how to use request handler to add a custom header to every request:
```go
proxy.HandleRequest = func(id int64, req *http.Request) *http.Response {
	req.Header.Add("custom", "myval")
	log.Printf("session: %v\n", id)
	return nil
}
```

## Response handler
The following example shows how to prevent access to a requests performed toward a specific host.

```go
proxy.HandleRequest = func(id int64, req *http.Request) *http.Response {
	if req.Host == "example.com" {
		return &http.Response{StatusCode: 500}
	}
	return nil
```

# A quick question ... why?
Because we can ... or we try.

