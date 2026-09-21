package relay

import (
	"bufio"
	"crypto/rand"
	"crypto/sha1"
	"crypto/tls"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Minimal RFC 6455 implementation (server accept + client dial + frame
// codec) for the /v1/realtime relay. No third-party dependency: the
// gateway relays frames, it does not need the full WebSocket surface.

// WebSocket opcodes (exported for tests and the realtime client).
const (
	WSOpContinuation = 0x0
	WSOpText         = 0x1
	WSOpBinary       = 0x2
	WSOpClose        = 0x8
	WSOpPing         = 0x9
	WSOpPong         = 0xA
)

const (
	wsOpContinuation = WSOpContinuation
	wsOpText         = WSOpText
	wsOpBinary       = WSOpBinary
	wsOpClose        = WSOpClose
	wsOpPing         = WSOpPing
	wsOpPong         = WSOpPong
)

// DialWebSocket connects to a websocket server (ws:// or wss://) and
// returns a frame-level client connection.
func DialWebSocket(rawURL string, header http.Header) (*WSConn, error) {
	return WSDial(rawURL, header)
}

const (
	wsGUID             = "258EAFA5-E914-47DA-95CA-C5AB0DC85B11"
	maxWSFramePayload  = 32 << 20
	wsHandshakeTimeout = 15 * time.Second
)

var errNotWebSocket = errors.New("not a websocket handshake")

// wsAcceptKey computes the Sec-WebSocket-Accept value of a client key.
func WSAcceptKey(clientKey string) string {
	sum := sha1.Sum([]byte(clientKey + wsGUID))
	return base64.StdEncoding.EncodeToString(sum[:])
}

// wsAccept upgrades a server request to a websocket connection. The caller
// must have verified the request deserves an upgrade; the 101 response is
// written here. Write errors before the upgrade are the caller's to emit.
func WSAccept(w http.ResponseWriter, r *http.Request) (*WSConn, error) {
	if !strings.EqualFold(r.Header.Get("Upgrade"), "websocket") ||
		!strings.Contains(strings.ToLower(r.Header.Get("Connection")), "upgrade") {
		return nil, errNotWebSocket
	}
	key := r.Header.Get("Sec-WebSocket-Key")
	if key == "" || r.Header.Get("Sec-WebSocket-Version") != "13" {
		return nil, errNotWebSocket
	}
	hijacker, ok := w.(http.Hijacker)
	if !ok {
		return nil, errors.New("server connection cannot be hijacked")
	}
	conn, buf, err := hijacker.Hijack()
	if err != nil {
		return nil, err
	}
	resp := "HTTP/1.1 101 Switching Protocols\r\n" +
		"Upgrade: websocket\r\n" +
		"Connection: Upgrade\r\n" +
		"Sec-WebSocket-Accept: " + WSAcceptKey(key) + "\r\n\r\n"
	if _, err := buf.WriteString(resp); err != nil {
		conn.Close()
		return nil, err
	}
	if err := buf.Flush(); err != nil {
		conn.Close()
		return nil, err
	}
	return &WSConn{conn: conn, reader: buf.Reader, client: true}, nil
}

// wsDial connects to a websocket server (ws:// or wss://) with extra
// headers. The returned connection masks frames as a client must.
func WSDial(rawURL string, header http.Header) (*WSConn, error) {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return nil, err
	}
	useTLS := parsed.Scheme == "wss" || parsed.Scheme == "https"
	host := parsed.Host
	if !strings.Contains(host, ":") {
		if useTLS {
			host += ":443"
		} else {
			host += ":80"
		}
	}
	var conn net.Conn
	if useTLS {
		conn, err = tls.Dial("tcp", host, &tls.Config{ServerName: parsed.Hostname()})
	} else {
		conn, err = net.Dial("tcp", host)
	}
	if err != nil {
		return nil, err
	}
	conn.SetDeadline(time.Now().Add(wsHandshakeTimeout))
	defer conn.SetDeadline(time.Time{})

	path := parsed.RequestURI()
	if path == "" {
		path = "/"
	}
	var keyBytes [16]byte
	if _, err := rand.Read(keyBytes[:]); err != nil {
		conn.Close()
		return nil, err
	}
	clientKey := base64.StdEncoding.EncodeToString(keyBytes[:])

	req := "GET " + path + " HTTP/1.1\r\n" +
		"Host: " + parsed.Host + "\r\n" +
		"Upgrade: websocket\r\n" +
		"Connection: Upgrade\r\n" +
		"Sec-WebSocket-Key: " + clientKey + "\r\n" +
		"Sec-WebSocket-Version: 13\r\n"
	for name, values := range header {
		for _, value := range values {
			req += name + ": " + value + "\r\n"
		}
	}
	req += "\r\n"
	if _, err := conn.Write([]byte(req)); err != nil {
		conn.Close()
		return nil, err
	}

	reader := bufio.NewReader(conn)
	httpReader := reader
	resp, err := http.ReadResponse(httpReader, &http.Request{Method: http.MethodGet})
	if err != nil {
		conn.Close()
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusSwitchingProtocols {
		conn.Close()
		return nil, fmt.Errorf("websocket upstream returned %d", resp.StatusCode)
	}
	if !strings.EqualFold(resp.Header.Get("Upgrade"), "websocket") {
		conn.Close()
		return nil, errors.New("upstream did not upgrade to websocket")
	}
	return &WSConn{conn: conn, reader: reader, client: false}, nil
}

// WSConn is one end of a websocket connection. client marks the side that
// must mask outgoing frames (the gateway dials upstream as a client).
type WSConn struct {
	conn   net.Conn
	reader *bufio.Reader
	client bool // true when this side speaks client-to-server framing
}

// WSFrame is one decoded frame.
type WSFrame struct {
	Opcode byte
	Data   []byte
}

// ReadFrame reads one frame (control frames included). Fragmented
// messages are reassembled. Exposed for tests and tooling.
func (c *WSConn) ReadFrame() (WSFrame, error) {
	var message []byte
	var messageOpcode byte
	for {
		frame, err := c.readSingleFrame()
		if err != nil {
			return WSFrame{}, err
		}
		switch frame.Opcode {
		case wsOpPing, wsOpPong, wsOpClose:
			return WSFrame{Opcode: frame.Opcode, Data: frame.Data}, nil
		case wsOpText, wsOpBinary:
			message = append(message, frame.Data...)
			messageOpcode = frame.Opcode
			if frame.fin {
				return WSFrame{Opcode: messageOpcode, Data: message}, nil
			}
		case wsOpContinuation:
			message = append(message, frame.Data...)
			if frame.fin {
				return WSFrame{Opcode: messageOpcode, Data: message}, nil
			}
		}
	}
}

type wsSingleFrame struct {
	fin    bool
	Opcode byte
	Data   []byte
}

func (c *WSConn) readSingleFrame() (wsSingleFrame, error) {
	var hdr [2]byte
	if _, err := io.ReadFull(c.reader, hdr[:]); err != nil {
		return wsSingleFrame{}, err
	}
	fin := hdr[0]&0x80 != 0
	opcode := hdr[0] & 0x0F
	masked := hdr[1]&0x80 != 0
	length := uint64(hdr[1] & 0x7F)
	switch length {
	case 126:
		var ext [2]byte
		if _, err := io.ReadFull(c.reader, ext[:]); err != nil {
			return wsSingleFrame{}, err
		}
		length = uint64(binary.BigEndian.Uint16(ext[:]))
	case 127:
		var ext [8]byte
		if _, err := io.ReadFull(c.reader, ext[:]); err != nil {
			return wsSingleFrame{}, err
		}
		length = binary.BigEndian.Uint64(ext[:])
	}
	if length > maxWSFramePayload {
		return wsSingleFrame{}, fmt.Errorf("websocket frame of %d bytes exceeds the limit", length)
	}
	var mask [4]byte
	if masked {
		if _, err := io.ReadFull(c.reader, mask[:]); err != nil {
			return wsSingleFrame{}, err
		}
	}
	payload := make([]byte, length)
	if _, err := io.ReadFull(c.reader, payload); err != nil {
		return wsSingleFrame{}, err
	}
	if masked {
		for i := range payload {
			payload[i] ^= mask[i%4]
		}
	}
	return wsSingleFrame{fin: fin, Opcode: opcode, Data: payload}, nil
}

// WriteFrame writes one frame. Frames from the client side are masked per
// RFC 6455 (the gateway masks what it sends upstream).
func (c *WSConn) WriteFrame(opcode byte, data []byte) error {
	if len(data) > maxWSFramePayload {
		return fmt.Errorf("websocket frame of %d bytes exceeds the limit", len(data))
	}
	var hdr []byte
	first := byte(0x80 | opcode) // FIN set, no fragmentation on writes
	// The gateway always masks what it sends to a server and never masks
	// what it sends to a client.
	maskBit := byte(0)
	var maskKey [4]byte
	if c.client {
		maskBit = 0x80
		if _, err := rand.Read(maskKey[:]); err != nil {
			return err
		}
	}
	hdr = append(hdr, first, 0)
	length := len(data)
	switch {
	case length < 126:
		hdr[1] = maskBit | byte(length)
	case length <= 0xFFFF:
		hdr[1] = maskBit | 126
		var ext [2]byte
		binary.BigEndian.PutUint16(ext[:], uint16(length))
		hdr = append(hdr, ext[:]...)
	default:
		hdr[1] = maskBit | 127
		var ext [8]byte
		binary.BigEndian.PutUint64(ext[:], uint64(length))
		hdr = append(hdr, ext[:]...)
	}
	if c.client {
		hdr = append(hdr, maskKey[:]...)
		data = append([]byte(nil), data...)
		for i := range data {
			data[i] ^= maskKey[i%4]
		}
	}
	if _, err := c.conn.Write(append(hdr, data...)); err != nil {
		return err
	}
	return nil
}

// Close frame payload: status code (1000) only.
func (c *WSConn) WriteClose() {
	_ = c.WriteFrame(wsOpClose, []byte{0x03, 0xE8})
}

func (c *WSConn) Close() error { return c.conn.Close() }

// SetReadDeadline extends the idle deadline (called on every frame).
func (c *WSConn) SetReadDeadline(d time.Duration) {
	_ = c.conn.SetReadDeadline(time.Now().Add(d))
}
