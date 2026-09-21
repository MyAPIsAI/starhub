package relay

import (
	"bufio"
	"crypto/sha1"
	"encoding/base64"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestWSAcceptKey(t *testing.T) {
	// The RFC 6455 example handshake value.
	if got := WSAcceptKey("dGhlIHNhbXBsZSBub25jZQ=="); got != "s3pPLMBiTxaQ9kYGzzhZRbK+xOo=" {
		t.Fatalf("accept key = %q", got)
	}
	// and the derivation is the documented sha1(key + GUID).
	sum := sha1.Sum([]byte("abc" + wsGUID))
	if got := WSAcceptKey("abc"); got != base64.StdEncoding.EncodeToString(sum[:]) {
		t.Fatal("accept key derivation mismatch")
	}
}

// pipeConnPair wires a client-side and server-side wsConn over net.Pipe.
func pipeConnPair() (client, server *WSConn) {
	a, b := net.Pipe()
	client = &WSConn{conn: a, reader: bufio.NewReader(a), client: true}
	server = &WSConn{conn: b, reader: bufio.NewReader(b), client: false}
	return
}

func TestWSFrameRoundTripClientToServer(t *testing.T) {
	// Client frames are masked; the server reader unmasks them.
	client, server := pipeConnPair()
	done := make(chan WSFrame, 1)
	go func() {
		server.SetReadDeadline(5 * time.Second)
		frame, err := server.ReadFrame()
		if err != nil {
			t.Errorf("server read: %v", err)
		}
		done <- frame
	}()
	payload := []byte(`{"type":"session.update"}`)
	if err := client.WriteFrame(wsOpText, payload); err != nil {
		t.Fatalf("write: %v", err)
	}
	select {
	case frame := <-done:
		if frame.Opcode != wsOpText || string(frame.Data) != string(payload) {
			t.Fatalf("frame = %d %q", frame.Opcode, frame.Data)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timed out")
	}
}

func TestWSFrameRoundTripServerToClient(t *testing.T) {
	// Server frames are unmasked; the client reader accepts them. The
	// payload exercises the 64-bit length branch (> 65535 bytes).
	client, server := pipeConnPair()
	done := make(chan WSFrame, 1)
	go func() {
		client.SetReadDeadline(5 * time.Second)
		frame, err := client.ReadFrame()
		if err != nil {
			t.Errorf("client read: %v", err)
		}
		done <- frame
	}()
	payload := bytes_64k()
	if err := server.WriteFrame(wsOpBinary, payload); err != nil {
		t.Fatalf("write: %v", err)
	}
	select {
	case frame := <-done:
		if frame.Opcode != wsOpBinary || len(frame.Data) != len(payload) {
			t.Fatalf("frame: %d bytes=%d", frame.Opcode, len(frame.Data))
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timed out")
	}
}

func TestWSFrameSizeLimit(t *testing.T) {
	_, server := pipeConnPair()
	huge := make([]byte, maxWSFramePayload+1)
	if err := server.WriteFrame(wsOpBinary, huge); err == nil {
		t.Fatal("oversized frame must be rejected on write")
	}
}

func TestWSDialAndAcceptEndToEnd(t *testing.T) {
	// A real socket round trip: our wsAccept (server) against our wsDial
	// (client), exercising the handshake and both masking directions.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := WSAccept(w, r)
		if err != nil {
			t.Errorf("accept: %v", err)
			return
		}
		defer conn.Close()
		conn.SetReadDeadline(5 * time.Second)
		frame, err := conn.ReadFrame()
		if err != nil {
			t.Errorf("server read: %v", err)
			return
		}
		if err := conn.WriteFrame(wsOpText, append([]byte("echo:"), frame.Data...)); err != nil {
			t.Errorf("server write: %v", err)
		}
		time.Sleep(50 * time.Millisecond) // let the echo drain to the client
	}))
	defer srv.Close()

	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http")
	conn, err := WSDial(wsURL, http.Header{"X-Test": []string{"1"}})
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()
	if err := conn.WriteFrame(wsOpText, []byte("ping")); err != nil {
		t.Fatalf("client write: %v", err)
	}
	conn.SetReadDeadline(5 * time.Second)
	frame, err := conn.ReadFrame()
	if err != nil {
		t.Fatalf("client read: %v", err)
	}
	if string(frame.Data) != "echo:ping" {
		t.Fatalf("echo wrong: %q", frame.Data)
	}
}

func TestWSAcceptRejectsNonUpgrade(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/v1/realtime", nil)
	rec := httptest.NewRecorder()
	if _, err := WSAccept(rec, req); err == nil {
		t.Fatal("non-websocket request must be rejected")
	}
}

func bytes_64k() []byte {
	out := make([]byte, 70000)
	for i := range out {
		out[i] = byte(i % 251)
	}
	return out
}
