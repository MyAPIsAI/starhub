package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

// messageBridge connects to an operator-managed mail/SMS adapter. A successful
// HTTP response must also acknowledge acceptance; delivery is tracked by the adapter.
type messageBridge struct {
	endpoint, token string
	client          *http.Client
}

func newMessageBridge(endpoint, token string) (*messageBridge, error) {
	u, err := url.Parse(endpoint)
	if err != nil || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || token == "" || strings.ContainsAny(token, "\r\n") {
		return nil, errors.New("invalid message bridge configuration")
	}
	if u.Scheme != "https" && !(u.Scheme == "http" && (u.Hostname() == "127.0.0.1" || u.Hostname() == "::1" || u.Hostname() == "localhost")) {
		return nil, errors.New("message bridge requires HTTPS or loopback HTTP")
	}
	return &messageBridge{endpoint, token, &http.Client{Timeout: 10 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}}, nil
}

func (p *messageBridge) send(payload any) error {
	return p.sendContext(context.Background(), payload)
}

func (p *messageBridge) sendContext(ctx context.Context, payload any) error {
	raw, err := json.Marshal(payload)
	if err != nil {
		return errors.New("message encoding failed")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.endpoint, bytes.NewReader(raw))
	if err != nil {
		return errors.New("message request failed")
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+p.token)
	resp, err := p.client.Do(req)
	if err != nil {
		return errors.New("message provider unavailable")
	}
	defer resp.Body.Close()
	var ack struct {
		Accepted bool   `json:"accepted"`
		ID       string `json:"id"`
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 || json.NewDecoder(io.LimitReader(resp.Body, 4096)).Decode(&ack) != nil || !ack.Accepted || ack.ID == "" {
		return errors.New("message provider did not acknowledge acceptance")
	}
	return nil
}

func (p *messageBridge) Send(phone, code string) error {
	return p.send(map[string]string{"kind": "sms_verification", "to": phone, "code": code})
}
func (p *messageBridge) SendVerification(email, code string) error {
	return p.send(map[string]string{"kind": "email_verification", "to": email, "code": code})
}
func (p *messageBridge) SendNotification(email, subject, body string) error {
	return p.send(map[string]string{"kind": "security_notification", "to": email, "subject": subject, "body": body})
}

// ConfigureMessageProviders leaves unconfigured transports disabled.
func ConfigureMessageProviders() error {
	var email *messageBridge
	var err error
	if endpoint := os.Getenv("STARHUB_EMAIL_BRIDGE_URL"); endpoint != "" {
		email, err = newMessageBridge(endpoint, os.Getenv("STARHUB_EMAIL_BRIDGE_TOKEN"))
		if err != nil {
			return err
		}
	}
	if err := ReloadSMSConfiguration(); err != nil {
		return err
	}
	if email != nil {
		SetEmailProvider(email)
	} else {
		SetEmailProvider(nil)
	}
	return nil
}
