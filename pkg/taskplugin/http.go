package taskplugin

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"time"
)

var client = &http.Client{Timeout: 30 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}

type Response struct {
	Status int             `json:"status"`
	Body   json.RawMessage `json:"body"`
}

// Do sends once, never follows redirects, and never exposes credentials to JS.
func (p *Plugin) Do(ctx context.Context, base, key string, descriptor Request, method string) (Response, error) {
	address, err := descriptor.URL(base, method)
	if err != nil {
		return Response{}, err
	}
	req, err := http.NewRequestWithContext(ctx, method, address, bytes.NewReader(descriptor.Body))
	if err != nil {
		return Response{}, errors.New("invalid plugin HTTP request")
	}
	for name, value := range descriptor.Headers {
		req.Header.Set(name, value)
	}
	req.Header.Set("Content-Type", "application/json")
	if p.Meta.Auth == "bearer" {
		req.Header.Set("Authorization", "Bearer "+key)
	} else {
		req.Header.Set("X-Api-Key", key)
	}
	resp, err := client.Do(req)
	if err != nil {
		return Response{}, errors.New("plugin upstream request failed; execution may be uncertain")
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, MaxJSON+1))
	if err != nil || len(body) > MaxJSON {
		return Response{Status: resp.StatusCode}, errors.New("plugin upstream response is unreadable or too large")
	}
	if !json.Valid(body) {
		return Response{Status: resp.StatusCode}, errors.New("plugin upstream response is not JSON")
	}
	return Response{Status: resp.StatusCode, Body: body}, nil
}
