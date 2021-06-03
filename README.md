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
        proxy := yves.GetDefault()

        // Listen on local port 8080
        log.Fatal(http.ListenAndServe(":8080", proxy))
}
```

## Requests handlers
TBD

## Responses handlers
TBD

# A quick question ... why?
Because we can ... or we try.

# TODOs
[ ] Introduce gomodules