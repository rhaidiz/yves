package yves

import (
	"bufio"
	"crypto/rand"
	"crypto/sha1"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"strings"
)

const (
	finalBit = 128 // 1 << 7
	rsv1Bit  = 64
	rsv2Bit  = 32
	rsv3Bit  = 16
	maskBit  = 128

	ContinuationFrame = 0
	// TextMessage denotes a text data message. The text message payload is
	// interpreted as UTF-8 encoded text data.
	TextMessage = 1

	// BinaryMessage denotes a binary data message.
	BinaryMessage = 2

	// CloseMessage denotes a close control message. The optional message
	// payload contains a numeric code and text. Use the FormatCloseMessage
	// function to format a close message payload.
	CloseMessage = 8

	// PingMessage denotes a ping control message. The optional message payload
	// is UTF-8 encoded text.
	PingMessage = 9

	// PongMessage denotes a pong control message. The optional message payload
	// is UTF-8 encoded text.
	PongMessage = 10
)

var keyGUID = []byte("258EAFA5-E914-47DA-95CA-C5AB0DC85B11")

// This is a websocket frame as per RFC6455 section-5.2
type WebsocketFragment struct {
	FinBit        bool
	Rsv1          bool
	Rsv2          bool
	Rsv3          bool
	OpCode        int
	MaskBit       bool
	PayloadLength uint64
	Key           []byte
	Data          []byte
}

var ErrorMaskKeyLength = errors.New("mask key length must be exactly 4 bytes")

func (frame *WebsocketFragment) Write(w io.Writer) error {
	var header []byte

	firstByte := byte(0)
	if frame.FinBit {
		firstByte |= finalBit
	}
	if frame.Rsv1 {
		firstByte |= rsv1Bit
	}
	if frame.Rsv2 {
		firstByte |= rsv2Bit
	}
	if frame.Rsv3 {
		firstByte |= rsv3Bit
	}
	firstByte |= byte(frame.OpCode)

	header = append(header, firstByte)

	secondByte := byte(0)
	if frame.MaskBit {
		secondByte |= maskBit
	}

	payloadLength := frame.PayloadLength
	if payloadLength < 126 {
		secondByte |= byte(payloadLength)
		header = append(header, secondByte)
	} else if payloadLength < 65536 {
		secondByte |= 126
		header = append(header, secondByte, byte(payloadLength>>8), byte(payloadLength))
	} else {
		secondByte |= 127
		header = append(header, secondByte,
			byte(payloadLength>>56), byte(payloadLength>>48),
			byte(payloadLength>>40), byte(payloadLength>>32),
			byte(payloadLength>>24), byte(payloadLength>>16),
			byte(payloadLength>>8), byte(payloadLength))
	}

	if frame.MaskBit {
		if len(frame.Key) != 4 {
			return ErrorMaskKeyLength
		}
		header = append(header, frame.Key...)
	}
	encryptedData := xorEncrypt(frame.Data, frame.Key)
	header = append(header, encryptedData...)

	if _, err := w.Write(header); err != nil {
		return errors.New("writing header to the writer")
	}

	return nil
}

func (proxy *Proxy) serveWebsocket(w http.ResponseWriter, req *http.Request, clientConn net.Conn) {
	targetURL := url.URL{Scheme: "ws", Host: req.Host, Path: req.URL.Path}

	targetConn, err := proxy.connectDial("tcp", targetURL.Host)
	if err != nil {
		return
	}
	defer targetConn.Close()

	// Perform handshake with client and remote server
	if err := proxy.websocketHandshake(req, targetConn, clientConn); err != nil {
		log.Printf("Websocket handshake error: %v", err)
		return
	}

	// Proxy ws connection
	proxy.proxyWebsocket(targetConn, clientConn)
}

func (proxy *Proxy) connectDial(network, addr string) (net.Conn, error) {
	return net.Dial(network, addr)
}

// complete the websocket handshare with the client and the target site.
// handshare with the client performed by swtiching the protocol to websocket and computing the value for sec-websocket-accept
// handshare with the server performed by asking protocol upgrading and checking that response is 101
func (proxy *Proxy) websocketHandshake(req *http.Request, targetSiteConn io.ReadWriter, clientConn io.ReadWriter) error {
	secWebsocketKey := req.Header["Sec-Websocket-Key"][0]
	secWebsocketAccept := computeAcceptKey(secWebsocketKey)

	response := &http.Response{
		Status:     "101 Switch Protocol",
		StatusCode: 101,
		Proto:      "HTTP/1.1",
		ProtoMajor: 1,
		ProtoMinor: 1,
		Header:     http.Header{},
	}
	response.Header.Add("Sec-Websocket-Accept", secWebsocketAccept)
	response.Header.Add("Connection", "Upgrade")
	response.Header.Add("Upgrade", "websocket")

	err := response.Write(clientConn)
	if err != nil {
		log.Printf("Error writing handshake response: %v", err)
		return err
	}

	request := &http.Request{
		Method: "GET",
		URL: &url.URL{
			Scheme: "ws",
			Host:   "127.0.0.1",
			Path:   "",
		},
		Header: http.Header{},
	}

	request.Header.Set("Connection", "Upgrade")
	request.Header.Set("Upgrade", "websocket")

	websocketKey := generateWebSocketKey()
	request.Header.Set("Sec-WebSocket-Key", websocketKey)
	request.Header.Set("Sec-WebSocket-Version", "13")

	request.Write(targetSiteConn)

	reader := bufio.NewReader(targetSiteConn)
	target_site_response, err := http.ReadResponse(reader, nil)
	if err != nil {
		return err
	}
	if target_site_response.StatusCode != 101 {
		return fmt.Errorf("upgrading connection")
	}

	return nil
}

// Helper function to generate a random Sec-WebSocket-Key
func generateWebSocketKey() string {
	// Generate 16 bytes of random data
	key := make([]byte, 16)
	rand.Read(key)

	// Base64 encode the random data
	return base64.StdEncoding.EncodeToString(key)
}

func (proxy *Proxy) proxyWebsocket(dest io.ReadWriter, source io.ReadWriter) {
	errChan := make(chan error, 2)

	// proxy from client to server
	go proxy.interceptWebsocket(dest, source, proxy.HandleWebSocRequest)
	// proxy from server to client
	go proxy.interceptWebsocket(source, dest, proxy.HandleWebSocResponse)
	<-errChan
}

func (proxy *Proxy) interceptWebsocket(dst io.Writer, src io.Reader, handler func(*WebsocketFragment) *WebsocketFragment) {
	scanner := bufio.NewReader(src)
	for {
		_, err := scanner.Peek(1)
		if err != nil {
			if err == io.EOF {
				continue
			}
		}
		websocFrag, err := ReadWebsocketFragment(scanner)
		if err != nil {
			log.Printf("error decoding websocket message %v\n", err)
			continue
		}

		if handler != nil {
			websocFrag = handler(websocFrag)
		}
		websocFrag.Write(dst)

		if err != nil {
			log.Printf("error writing websocket message %v\n", err)

		}
	}
}

func ReadWebsocketFragment(b *bufio.Reader) (*WebsocketFragment, error) {
	websocFrame := &WebsocketFragment{}
	byteRead, err := b.ReadByte()
	if err != nil {
		return nil, err
	}

	websocFrame.FinBit = int(byteRead&finalBit) != 0 // 1 bit
	websocFrame.Rsv1 = int(byteRead&rsv1Bit) != 0    // 1 bit
	websocFrame.Rsv2 = int(byteRead&rsv2Bit) != 0    // 1 bit
	websocFrame.Rsv3 = int(byteRead&rsv3Bit) != 0    // 1 bit
	websocFrame.OpCode = int(byteRead & 0x0f)        // 4 bit

	byteRead, err = b.ReadByte()
	if err != nil {
		return nil, err
	}

	websocFrame.MaskBit = int(byteRead&maskBit) != 0
	payloadLength := uint64(byteRead & 0x7f)

	var lenn []byte
	switch payloadLength {
	case 126:
		lenn1, err := b.ReadByte()
		if err != nil {
			return nil, err
		}
		lenn2, err := b.ReadByte()
		if err != nil {
			return nil, err
		}
		lenn = append(lenn, lenn1, lenn2)
		payloadLength = uint64(binary.BigEndian.Uint16(lenn))
	case 127:
		for i := 0; i < 8; i++ {
			l, err := b.ReadByte()
			if err != nil {
				return nil, err
			}
			lenn = append(lenn, l)
		}
		payloadLength = uint64(binary.BigEndian.Uint64(lenn))
	}
	websocFrame.PayloadLength = payloadLength

	key := make([]byte, 4)
	if websocFrame.MaskBit {
		for i := 0; i < 4; i++ {
			k, err := b.ReadByte()
			if err != nil {
				return nil, err
			}
			key[i] = k
		}
	}
	websocFrame.Key = key

	data := make([]byte, 0)
	for i := 0; uint64(i) < payloadLength; i++ {
		c, err := b.ReadByte()
		if err != nil {
			return nil, err
		}
		data = append(data, c)
	}
	websocFrame.Data = xorEncrypt(data, websocFrame.Key)
	return websocFrame, nil

}

func headerContains(header http.Header, name string, value string) bool {
	for _, v := range header[name] {
		for _, s := range strings.Split(v, ",") {
			if strings.EqualFold(value, strings.TrimSpace(s)) {
				return true
			}
		}
	}
	return false
}

func isWebSocketRequest(r *http.Request) bool {
	return headerContains(r.Header, "Connection", "upgrade") &&
		headerContains(r.Header, "Upgrade", "websocket")
}

func computeAcceptKey(key string) string {
	// Create a new SHA-1 hash
	h := sha1.New()
	// Write a given key from client
	h.Write([]byte(key))
	// Concatenate the key with the GUID
	h.Write(keyGUID)
	// Base64-encoded
	return base64.StdEncoding.EncodeToString(h.Sum(nil))
}

func xorEncrypt(data, key []byte) []byte {
	encrypted := make([]byte, len(data))
	keyLen := len(key)
	for i, b := range data {
		encrypted[i] = b ^ key[i%keyLen]
	}
	return encrypted
}
