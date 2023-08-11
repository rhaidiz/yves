package yves

import (
	"bufio"
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net"
	"net/http"
	"net/url"
	"sync"
	"time"
)

var okHeader = "HTTP/1.1 200 OK\r\n\r\n"

type Proxy struct {
	// HttpClient is a working HTTP Client
	HttpClient *http.Client

	// Tr is the transport used by the HttpClient
	Tr *http.Transport

	// HandleRequest is a function that is executed upon receving a request
	HandleRequest func(int64, *http.Request) *http.Response

	// HandleResponse is a function that is executed when a response is being sent back
	HandleResponse func(int64, *http.Request, *http.Response)

	// Session is used to count the number of requests received
	// so that it is possible to correlate requests and responses from the handlers.
	session      int64
	sessionMutex sync.Mutex

	// CaKey and CaCert are, respectively the proxy TLS private
	// key and certificate in PEM format.
	CaKey  []byte
	CaCert []byte

	HandleWebSocRequest  func(websoc *WebsocketFragment) *WebsocketFragment
	HandleWebSocResponse func(websoc *WebsocketFragment) *WebsocketFragment
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

	//Bleah: Needed for HTTPS
	//And what about HTTP2?
	// this is the connection with the client
	clientConn, _, err := hijacker.Hijack()
	defer clientConn.Close()

	if err != nil {
		HttpError(wrt, err.Error(), http.StatusInternalServerError)
		return
	}

	fmt.Printf("%v\n", req)
	if req.Method != http.MethodConnect {
		// this is a plaintext HTTP connection
		reqClone := req.Clone(context.TODO())
		if err != nil {
			HttpError(wrt, err.Error(), http.StatusInternalServerError)
			return
		}

		// Forward the request to the remote host
		// RequestURI will contain the Request Target
		// https://datatracker.ietf.org/doc/html/rfc7230#section-5.3.2
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
		// Some usefull documentation
		// https://datatracker.ietf.org/doc/html/rfc2817#section-5.2
		// For being compliant with the RFC, the proxy should first
		// perform a dial to the remote destination host and *then* send
		// a 200OK to the client. However, I have no idea how to do this
		// while leveraging the convinience of Transport provided by Go.
		// So for know, I will knowingly violate the RFC.

		// Save the destinationHost along with the scheme.
		destinationHost := fmt.Sprintf("https://%s", req.RequestURI)

		// Answer with a 200OK to the client.
		clientConn.Write([]byte(okHeader))

		// check if destination speaks TLS
		conf := &tls.Config{
			InsecureSkipVerify: true,
		}

		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		d := tls.Dialer{
			Config: conf,
		}
		_, err := d.DialContext(ctx, "tcp", req.RequestURI)
		cancel() // why am I calling the cancel function?
		if err != nil {
			//defer conn.Close()
			// not a TLS connection, go with raw tcp for now
			// I'm assuming that if I cannot establish a TLS connection with
			// the remote server maybe this is a plaintext websocket connection
			clientTlsReader := bufio.NewReader(clientConn)
			req, err := http.ReadRequest(clientTlsReader)
			if err != nil {
				log.Println("Not an HTTP request")
			}
			if isWebSocketRequest(req) {
				p.serveWebsocket(wrt, req, clientConn, false)
			}
			return

		} else {
			// a TLS connection

			// Start a TLS connection with the client.
			clientConn = p.startTlsWithClient(clientConn)
			defer clientConn.Close()

			clientTlsReader := bufio.NewReader(clientConn)
			for !isEob(clientTlsReader) {

				reqClone := req.Clone(context.TODO())
				if err != nil {
					HttpError(wrt, err.Error(), http.StatusInternalServerError)
					return
				}

				req, err := http.ReadRequest(clientTlsReader)
				if err != nil {
					// Assume this is a HTTPS connection
					//clientConnTls = p.startTlsWithClient(clientConn)
					log.Println("Not an HTTP request")
				} else {

					if isWebSocketRequest(req) {
						p.serveWebsocket(wrt, req, clientConn, true)
					}
					return
				}

				resp, err := p.forwardReq(ctx, req, destinationHost)
				if err != nil {
					HttpError(clientConn, err.Error(), http.StatusInternalServerError)
					return
				}
				// Do I need to have a write buffer for the connection with the client??
				error := p.forwardResp(ctx, resp, clientConn, reqClone)
				if error != nil {
					HttpError(clientConn, error.Error(), http.StatusInternalServerError)
					return
				}
				return
			}
			return
		}
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

	clientRequest.RequestURI = ""

	u, err := url.Parse(destinationHost)
	if err != nil {
		return nil, err
	}

	clientRequest.URL.Scheme = u.Scheme
	clientRequest.URL.Host = u.Host
	return p.HttpClient.Do(clientRequest)
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

func NewProxy() *Proxy {
	p := &Proxy{}
	certs = make(map[string]*tls.Certificate)
	// By default skip TLS verification
	p.Tr = &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
	}
	// By default:
	// - do not follow redirection;
	// - set a 10 seconds timeout
	cl := &http.Client{
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
		Transport: p.Tr,
		Timeout:   time.Second * 10}
	p.HttpClient = cl
	if p.CaCert == nil || p.CaKey == nil {
		p.CaCert = caCert
		p.CaKey = caKey
	}
	return p
}

// startTlsWithClient starts a TLS connection with the client.
func (p *Proxy) startTlsWithClient(down net.Conn) net.Conn {

	tlfConf := new(tls.Config)
	// https://pkg.go.dev/crypto/tls#Config
	// GetCertificate returns a Certificate based on the given
	// ClientHelloInfo. It will only be called if the client supplies SNI
	// information or if Certificates is empty.
	tlfConf.GetCertificate = func(hello *tls.ClientHelloInfo) (*tls.Certificate, error) {
		// get CA key pair
		CA, err := tls.X509KeyPair(p.CaCert, p.CaKey)
		if err != nil {
			log.Fatalf("Cannot parse provided CA key pair %s\n", err)
		}
		// get CA certificate
		CA.Leaf, err = x509.ParseCertificate(CA.Certificate[0])
		if err != nil {
			log.Fatalf("Cannot parse CA certificate: %s\n", err)
		}
		return getCert(CA, hello.ServerName)
	}

	// perform a TLS connection with the client.
	c := tls.Server(down, tlfConf)
	if err := c.Handshake(); err != nil {
		log.Printf("Server Handshake error: %v\n", err)
	}
	return c
}

// isEob check is there's something else to read from the buffer.
func isEob(r *bufio.Reader) bool {
	_, err := r.Peek(1)
	if err != nil {
		return true
	}
	return false
}
