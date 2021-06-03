package yves

import (
	"context"
	"crypto/tls"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"time"
)

type Proxy struct {
	// The Transport used by the proxy and the remote host
	Tr *http.Transport

	// The TLS configuration to use for remote connections
	TlsConfig *tls.Config
}

func (p *Proxy) ServeHTTP(wrt http.ResponseWriter, req *http.Request) {
	// hijack the connection with the client
	hijacker, ok := wrt.(http.Hijacker)

	if !ok {
		http.Error(wrt, "Hijacking not supported", http.StatusInternalServerError)
		return
	}

	// this is the connection with the client
	clientStream, _, err := hijacker.Hijack()
	defer clientStream.Close()

	if err != nil {
		http.Error(wrt, err.Error(), http.StatusInternalServerError)
		return
	}

	if req.Method != http.MethodConnect {
		// this is a plaintext HTTP connection

		reqClone := req.Clone(context.TODO())
		if err != nil {
			http.Error(wrt, err.Error(), http.StatusInternalServerError)
			return
		}

		// Forward the request to the remote host
		resp, err := p.forwardReq(req, req.RequestURI)

		if err != nil {
			http.Error(wrt, err.Error(), http.StatusInternalServerError)
			return
		}

		// forward the response back to the client
		p.forwardResp(resp, clientStream, reqClone)

	} else {
		// CONNECT not yet supported
	}
}

func (p *Proxy) forwardReq(req *http.Request, remoteHost string) (*http.Response, error) {

	timeout := time.Duration(5 * time.Second)

	tr := &http.Transport{}

	if p.Tr != nil {
		tr = p.Tr
	}

	cl := &http.Client{Transport: tr, Timeout: timeout}
	req.RequestURI = ""

	u, err := url.Parse(remoteHost)
	if err != nil {
		return nil, err
	}

	req.URL = u
	return cl.Do(req)
}

// TODO: maybe the forwardResp can be better?
func (p *Proxy) forwardResp(resp *http.Response, down net.Conn, req *http.Request) (int, error) {
	dump, _ := httputil.DumpResponse(resp, true)
	defer down.Close()
	return down.Write(dump)
}
