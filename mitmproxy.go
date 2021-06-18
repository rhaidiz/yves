package yves

import (
	"bytes"
	"context"
	"crypto/tls"
	"io"
	"io/ioutil"
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
		HttpError(wrt, "Hijacking not supported", http.StatusInternalServerError)
		return
	}

	//Bleah: Needed only for HTTPS
	//And what about HTTP2?
	// this is the connection with the client
	clientConn, _, err := hijacker.Hijack()
	defer clientConn.Close()

	if err != nil {
		HttpError(wrt, err.Error(), http.StatusInternalServerError)
		return
	}

	if req.Method != http.MethodConnect {
		// this is a plaintext HTTP connection
		reqClone := req.Clone(context.TODO())
		if err != nil {
			HttpError(wrt, err.Error(), http.StatusInternalServerError)
			return
		}

		// Forward the request to the remote host
		resp, err := p.forwardReq(req, req.RequestURI)

		if err != nil {
			HttpError(wrt, err.Error(), http.StatusInternalServerError)
			return
		}

		// forward the response back to the client
		_, error := p.forwardResp(resp, clientConn, reqClone)
		if error != nil {
			HttpError(clientConn, error.Error(), http.StatusInternalServerError)
			return
		}

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
	dump, error := httputil.DumpResponse(resp, true)
	if error != nil {
		return 0, error
	}
	return down.Write(dump)
}

func HttpError(conn io.Writer, er string, code int) {
	rsp := &http.Response{
		ProtoMajor: 1,
		ProtoMinor: 1,
		StatusCode: code,
		Header:     make(map[string][]string),
		Body:       ioutil.NopCloser(bytes.NewBufferString(er)),
	}
	rsp.Header.Add("Content-Type", "text/plain; charset=utf-8")
	rsp.Header.Add("X-Content-Type-Options", "nosniff")
	dump, error := httputil.DumpResponse(rsp, true)
	if error != nil {
		//TODO: add error handling here
	}
	conn.Write(dump)
}
