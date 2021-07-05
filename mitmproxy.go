package yves

import (
	"bytes"
	"context"
	"crypto/tls"
	"io"
	"io/ioutil"
	"net/http"
	"net/url"
	"sync"
	"time"
)

type Proxy struct {
	// The Transport used by the proxy and the remote host
	httpClient *http.Client

	// The TLS configuration to use for remote connections
	TlsConfig *tls.Config

	// Request handler
	HandleRequest func(int64, *http.Request) *http.Response

	// Response handler
	HandleResponse func(int64, *http.Request, *http.Response)

	// Session is used to count the number of requests received
	// so that it is possible to correlate requests and responses from the handlers
	session      int64
	sessionMutex sync.Mutex
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
	p.sessionMutex.Lock()
	ctx := context.WithValue(context.Background(), "session", p.session)
	p.session = p.session + 1
	p.sessionMutex.Unlock()
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
		resp, err := p.forwardReq(ctx, req, req.RequestURI)

		if err != nil {
			HttpError(wrt, err.Error(), http.StatusInternalServerError)
			return
		}

		// forward the response back to the client
		error := p.forwardResp(ctx, resp, clientConn, reqClone)
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

// Takes the client request, eventually modifies it and sends it to the intended destination host
func (p *Proxy) forwardReq(ctx context.Context, clientRequest *http.Request, destinationHost string) (*http.Response, error) {

	if p.HandleRequest != nil {
		// call to HandleRequest
		hResp := p.HandleRequest(ctx.Value("session").(int64), clientRequest)
		if hResp != nil {
			return hResp, nil
		}
	}
	//some things are missing, like timeouts etc, refer to blogpost (?)

	clientRequest.RequestURI = ""

	u, err := url.Parse(destinationHost)
	if err != nil {
		return nil, err
	}

	clientRequest.URL = u
	return p.httpClient.Do(clientRequest)
}

func (p *Proxy) forwardResp(ctx context.Context, resp *http.Response, down io.Writer, req *http.Request) error {
	if p.HandleResponse != nil {
		p.HandleResponse(ctx.Value("session").(int64), req, resp)
	}
	return resp.Write(down)
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
	rsp.Write(conn)
}
