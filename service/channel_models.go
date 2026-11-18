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

	"starhub/pkg/taskplugin"
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
	return FetchChannelModelsWithAuth(ctx, baseURL, key, "bearer")
}

// FetchChannelModelsWithAuth uses the authentication scheme declared by the adapter.
func FetchChannelModelsWithAuth(ctx context.Context, baseURL, key, auth string) ([]string, error) {
	base, err := NormalizeChannelModelsBaseURL(baseURL)
	if err != nil {
		return nil, err
	}
	return fetchChannelModels(ctx, base+"/v1/models", key, auth)
}

// NormalizeTaskPluginModelsBaseURL preserves the adapter's configured path prefix.
func NormalizeTaskPluginModelsBaseURL(raw string) (string, error) {
	base := strings.TrimRight(strings.TrimSpace(raw), "/")
	if _, err := (taskplugin.Request{Method: http.MethodGet, Path: "/"}).URL(base, http.MethodGet); err != nil {
		return "", &ChannelModelsError{http.StatusBadRequest, "invalid_models_base_url"}
	}
	u, _ := url.Parse(base)
	u.Host = strings.ToLower(u.Host)
	return u.String(), nil
}

// FetchTaskPluginChannelModels keeps an existing API version prefix intact.
func FetchTaskPluginChannelModels(ctx context.Context, baseURL, key, auth string) ([]string, error) {
	base, err := NormalizeTaskPluginModelsBaseURL(baseURL)
	if err != nil {
		return nil, err
	}
	path := "/v1/models"
	if strings.HasSuffix(base, "/v1") {
		path = "/models"
	}
	endpoint, err := (taskplugin.Request{Method: http.MethodGet, Path: path}).URL(base, http.MethodGet)
	if err != nil {
		return nil, &ChannelModelsError{http.StatusBadRequest, "invalid_models_base_url"}
	}
	return fetchChannelModels(ctx, endpoint, key, auth)
}

func fetchChannelModels(ctx context.Context, endpoint, key, auth string) ([]string, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, &ChannelModelsError{http.StatusBadRequest, "invalid_models_base_url"}
	}
	request.Header.Set("Accept", "application/json")
	switch auth {
	case "bearer":
		request.Header.Set("Authorization", "Bearer "+key)
	case "x-api-key":
		request.Header.Set("x-api-key", key)
	default:
		return nil, &ChannelModelsError{http.StatusBadRequest, "models_plugin_unavailable"}
	}
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
