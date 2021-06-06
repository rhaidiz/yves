package yves

import (
	"context"
	"crypto/tls"
	"io"
	"net/http"
	"net/http/httputil"
	"net/url"
	"time"
)

type Proxy struct {
	// The Transport used by the proxy and the remote host
	httpClient *http.Client

	// The TLS configuration to use for remote connections
	TlsConfig *tls.Config

	proxyFunctions []ProxyFunction
}

type ProxyFunction struct {
	// returns if the request needs to be dropped, if there is a response hook and the new request
	Func    func(*http.Request) (*http.Request, func(*http.Request, *http.Response) *http.Response)
	Order   uint
	Enabled bool
}

func GetDefault() *Proxy {
	cl := &http.Client{
		Transport: &http.Transport{},
		Timeout:   time.Second * 10}

	return &Proxy{httpClient: cl}
}

func GetWithCustomClient(cl *http.Client) *Proxy {
	return &Proxy{httpClient: cl}
}

func (p *Proxy) ServeHTTP(wrt http.ResponseWriter, req *http.Request) {
	// hijack the connection with the client
	hijacker, ok := wrt.(http.Hijacker)

	if !ok {
		http.Error(wrt, "Hijacking not supported", http.StatusInternalServerError)
		return
	}

	//Bleah: Needed only for HTTPS
	//And what about HTTP2?
	// this is the connection with the client
	clientConn, _, err := hijacker.Hijack()
	defer clientConn.Close()

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

		var respoonseHandlers []func(*http.Request, *http.Response) *http.Response

		//Hijack request here
		for i, r := range p.proxyFunctions {
			if !r.Enabled {
				continue
			}
			//Is this even needed? Maybe to sort
			r.Order = uint(i + 1)
			reqClone, responseFunc := r.Func(reqClone)

			if reqClone == nil {
				//Abort
				clientConn.Close()
				return
			}

			if responseFunc != nil {
				//Add to response array
				respoonseHandlers = append(respoonseHandlers, responseFunc)
			}
		}

		// Forward the request to the remote host
		resp, err := p.forwardReq(req, req.RequestURI)

		if err != nil {
			http.Error(wrt, err.Error(), http.StatusInternalServerError)
			return
		}

		//Hijack response here
		for _, rh := range respoonseHandlers {
			resp = rh(reqClone, resp)
			if resp == nil {
				//Abort
				clientConn.Close()
				return
			}
		}

		// forward the response back to the client
		p.forwardResp(resp, clientConn, reqClone)

	} else {
		// CONNECT not yet supported
		// dial to remote host
		// send 200 ok to client
		// startTLS with client
		// read request from connection with client
		// forward request
		// forward response
	}
}

//Takes the client request, eventually modifies it and sends it to the intended destination host
func (p *Proxy) forwardReq(clientRequest *http.Request, destinationHost string) (*http.Response, error) {

	//some things are missing, like timeouts etc, refer to blogpost (?)

	//Custom requests??
	clientRequest.RequestURI = ""

	u, err := url.Parse(destinationHost)
	if err != nil {
		return nil, err
	}

	clientRequest.URL = u
	return p.httpClient.Do(clientRequest)
}

// TODO: maybe the forwardResp can be better?
//forwardResp takes the origin reply, eventually modifies it and returns it to the client
func (p *Proxy) forwardResp(resp *http.Response, down io.Writer, req *http.Request) (int, error) {
	dump, _ := httputil.DumpResponse(resp, true)
	return down.Write(dump)
}
