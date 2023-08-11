package main

import (
	"log"
	"net/http"
	"os"

	"github.com/rhaidiz/yves"
)

func main() {

	caCert, err := os.ReadFile("demo.pem")
	if err != nil {
		log.Fatal(err)
	}
	caKey, err := os.ReadFile("demo.key.pem")
	if err != nil {
		log.Fatal(err)
	}

	proxy := yves.NewProxy()
	proxy.CaCert = caCert
	proxy.CaKey = caKey

	// Listen on local port 8080
	log.Fatal(http.ListenAndServe("127.0.0.1:8080", proxy))

}
