package service

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
	"unicode"
)

// ChannelModelsError contains only controlled, credential-free API messages.
type ChannelModelsError struct {
	Status int
	Code   string
}

func (e *ChannelModelsError) Error() string { return e.Code }

// NormalizeChannelModelsBaseURL follows the OpenAI relay's root URL convention.
func NormalizeChannelModelsBaseURL(raw string) (string, error) {
	base := strings.TrimRight(strings.TrimSpace(raw), "/")
	if base == "" {
		base = "https://api.openai.com"
	}
	u, err := url.Parse(base)
	if err != nil || u.Hostname() == "" || (u.Scheme != "http" && u.Scheme != "https") ||
		u.User != nil || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" || u.RawPath != "" {
		return "", &ChannelModelsError{http.StatusBadRequest, "invalid_models_base_url"}
	}
	if strings.HasSuffix(u.Path, "/v1") {
		return "", &ChannelModelsError{http.StatusBadRequest, "models_base_url_contains_v1"}
	}
	u.Host = strings.ToLower(u.Host)
	return u.String(), nil
}

// FetchChannelModels reads a catalog without probing or changing channel state.
func FetchChannelModels(ctx context.Context, baseURL, key string) ([]string, error) {
	base, err := NormalizeChannelModelsBaseURL(baseURL)
	if err != nil {
		return nil, err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, base+"/v1/models", nil)
	if err != nil {
		return nil, &ChannelModelsError{http.StatusBadRequest, "invalid_models_base_url"}
	}
	request.Header.Set("Accept", "application/json")
	request.Header.Set("Authorization", "Bearer "+key)
	client := &http.Client{
		Timeout:       10 * time.Second,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
	response, err := client.Do(request)
	if err != nil {
		var networkError net.Error
		if errors.Is(err, context.DeadlineExceeded) || (errors.As(err, &networkError) && networkError.Timeout()) {
			return nil, &ChannelModelsError{http.StatusGatewayTimeout, "upstream_models_timeout"}
		}
		return nil, &ChannelModelsError{http.StatusBadGateway, "upstream_models_network_error"}
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		code := "upstream_models_failed"
		switch response.StatusCode {
		case http.StatusUnauthorized, http.StatusForbidden:
			code = "upstream_models_authentication_failed"
		case http.StatusNotFound:
			code = "upstream_models_not_found"
		case http.StatusTooManyRequests:
			code = "upstream_models_rate_limited"
		default:
			if response.StatusCode >= 300 && response.StatusCode < 400 {
				code = "upstream_models_redirect_refused"
			}
		}
		return nil, &ChannelModelsError{http.StatusBadGateway, code}
	}
	const maxBytes = 2 << 20
	body, err := io.ReadAll(io.LimitReader(response.Body, maxBytes+1))
	if err != nil {
		return nil, &ChannelModelsError{http.StatusBadGateway, "upstream_models_read_failed"}
	}
	if len(body) > maxBytes {
		return nil, &ChannelModelsError{http.StatusBadGateway, "upstream_models_too_large"}
	}
	var result struct {
		Data *[]struct {
			ID string `json:"id"`
		} `json:"data"`
		HasMore       bool   `json:"has_more"`
		NextCursor    string `json:"next_cursor"`
		NextPageToken string `json:"next_page_token"`
		Next          string `json:"next"`
	}
	if err := json.Unmarshal(body, &result); err != nil || result.Data == nil {
		return nil, &ChannelModelsError{http.StatusBadGateway, "upstream_models_invalid_response"}
	}
	if result.HasMore || result.NextCursor != "" || result.NextPageToken != "" || result.Next != "" {
		return nil, &ChannelModelsError{http.StatusBadGateway, "upstream_models_pagination_unsupported"}
	}
	if len(*result.Data) > 5000 {
		return nil, &ChannelModelsError{http.StatusBadGateway, "upstream_models_too_large"}
	}
	models := make([]string, 0, len(*result.Data))
	seen := make(map[string]bool)
	for _, item := range *result.Data {
		id := strings.TrimSpace(item.ID)
		if id == "" {
			continue
		}
		if len(id) > 128 || strings.Contains(id, ",") || strings.ContainsFunc(id, unicode.IsControl) {
			return nil, &ChannelModelsError{http.StatusBadGateway, "upstream_models_invalid_id"}
		}
		if !seen[id] {
			models = append(models, id)
			seen[id] = true
		}
	}
	return models, nil
}
