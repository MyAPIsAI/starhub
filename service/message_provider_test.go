package service

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestMessageBridgeRequiresAcknowledgmentAndDoesNotFollowRedirects(t *testing.T) {
	status, body := 200, `{"accepted":true,"id":"provider-1"}`
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer test-secret" {
			t.Error("missing provider authorization")
		}
		w.Header().Set("Location", "/redirect")
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	defer s.Close()
	p, err := newMessageBridge(s.URL, "test-secret")
	if err != nil {
		t.Fatal(err)
	}
	if err := p.SendVerification("test@example.test", "123456"); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		status int
		body   string
	}{{200, `{}`}, {200, `{"accepted":false,"id":"x"}`}, {500, `{"accepted":true,"id":"x"}`}, {302, `{}`}} {
		status, body = tc.status, tc.body
		if p.Send("+12025550100", "123456") == nil {
			t.Fatal("unacknowledged message accepted")
		}
	}
	if _, err := newMessageBridge("http://provider.example/send", "secret"); err == nil {
		t.Fatal("public plaintext provider allowed")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := p.sendContext(ctx, map[string]string{"kind": "security_notification"}); err == nil {
		t.Fatal("cancelled provider request was sent")
	}
}
