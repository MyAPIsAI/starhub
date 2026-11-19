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
	"starhub/relay/channel/ark"
	"starhub/relay/channel/compatible"
)

// ChannelModelsError contains only controlled, credential-free API messages.
type ChannelModelsError struct {
	Status     int
	Code       string
	Diagnostic *ChannelModelsDiagnostic
}

type ChannelModelsDiagnostic struct {
	UpstreamStatus int    `json:"upstream_status"`
	ProviderCode   string `json:"provider_code,omitempty"`
	RequestID      string `json:"request_id,omitempty"`
	UpstreamHost   string `json:"upstream_host,omitempty"`
	CatalogPath    string `json:"catalog_path,omitempty"`
	KeySource      string `json:"key_source,omitempty"`
	KeyIndex       *int   `json:"key_index,omitempty"`
}

type ChannelModelInfo struct {
	ID     string `json:"id"`
	Domain string `json:"domain,omitempty"`
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
		return "", &ChannelModelsError{Status: http.StatusBadRequest, Code: "invalid_models_base_url"}
	}
	if strings.HasSuffix(u.Path, "/v1") {
		return "", &ChannelModelsError{Status: http.StatusBadRequest, Code: "models_base_url_contains_v1"}
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

// NormalizeCompatibleModelsBaseURL shares the provider's inference prefix.
func NormalizeCompatibleModelsBaseURL(channelType int, raw string) (string, error) {
	adaptor, ok := compatible.ForType(channelType)
	if !ok || !adaptor.HasModelCatalog() {
		return "", &ChannelModelsError{Status: http.StatusBadRequest, Code: "models_channel_type_unsupported"}
	}
	return NormalizeTaskPluginModelsBaseURL(adaptor.APIBaseURL(raw))
}

func FetchCompatibleChannelModels(ctx context.Context, channelType int, baseURL, key, auth string) ([]string, error) {
	base, err := NormalizeCompatibleModelsBaseURL(channelType, baseURL)
	if err != nil {
		return nil, err
	}
	return fetchChannelModels(ctx, base+"/models", key, auth)
}

// NormalizeArkModelsBaseURL uses the same API prefix as inference requests.
func NormalizeArkModelsBaseURL(raw string) (string, error) {
	return NormalizeTaskPluginModelsBaseURL(ark.APIBaseURL(raw))
}

func FetchArkChannelModels(ctx context.Context, baseURL, key, auth string) ([]string, error) {
	catalog, err := FetchArkChannelModelCatalog(ctx, baseURL, key, auth)
	return channelModelIDs(catalog), err
}

func FetchArkChannelModelCatalog(ctx context.Context, baseURL, key, auth string) ([]ChannelModelInfo, error) {
	key = strings.TrimSpace(key)
	if key == "" || strings.HasPrefix(strings.ToLower(key), "bearer ") || strings.ContainsFunc(key, unicode.IsSpace) {
		return nil, &ChannelModelsError{Status: http.StatusBadRequest, Code: "models_key_format_invalid"}
	}
	base, err := NormalizeArkModelsBaseURL(baseURL)
	if err != nil {
		return nil, err
	}
	return fetchChannelModelCatalog(ctx, base+"/models", key, auth)
}

// NormalizeTaskPluginModelsBaseURL preserves the adapter's configured path prefix.
func NormalizeTaskPluginModelsBaseURL(raw string) (string, error) {
	base := strings.TrimRight(strings.TrimSpace(raw), "/")
	if _, err := (taskplugin.Request{Method: http.MethodGet, Path: "/"}).URL(base, http.MethodGet); err != nil {
		return "", &ChannelModelsError{Status: http.StatusBadRequest, Code: "invalid_models_base_url"}
	}
	u, _ := url.Parse(base)
	u.Host = strings.ToLower(u.Host)
	return u.String(), nil
}

// FetchTaskPluginChannelModels reads an OpenAI-shaped catalog beneath the adapter's configured base.
func FetchTaskPluginChannelModels(ctx context.Context, baseURL, key, auth, path string) ([]string, error) {
	base, err := NormalizeTaskPluginModelsBaseURL(baseURL)
	if err != nil {
		return nil, err
	}
	if path == "" {
		path = "/v1/models"
		if strings.HasSuffix(base, "/v1") {
			path = "/models"
		}
	}
	endpoint, err := (taskplugin.Request{Method: http.MethodGet, Path: path}).URL(base, http.MethodGet)
	if err != nil {
		return nil, &ChannelModelsError{Status: http.StatusBadRequest, Code: "invalid_models_base_url"}
	}
	return fetchChannelModels(ctx, endpoint, key, auth)
}

func fetchChannelModels(ctx context.Context, endpoint, key, auth string) ([]string, error) {
	catalog, err := fetchChannelModelCatalog(ctx, endpoint, key, auth)
	return channelModelIDs(catalog), err
}

func channelModelIDs(catalog []ChannelModelInfo) []string {
	ids := make([]string, 0, len(catalog))
	for _, entry := range catalog {
		ids = append(ids, entry.ID)
	}
	return ids
}

func fetchChannelModelCatalog(ctx context.Context, endpoint, key, auth string) ([]ChannelModelInfo, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, &ChannelModelsError{Status: http.StatusBadRequest, Code: "invalid_models_base_url"}
	}
	request.Header.Set("Accept", "application/json")
	switch auth {
	case "bearer":
		request.Header.Set("Authorization", "Bearer "+key)
	case "x-api-key":
		request.Header.Set("x-api-key", key)
	default:
		return nil, &ChannelModelsError{Status: http.StatusBadRequest, Code: "models_plugin_unavailable"}
	}
	client := &http.Client{
		Timeout:       10 * time.Second,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
	response, err := client.Do(request)
	if err != nil {
		var networkError net.Error
		if errors.Is(err, context.DeadlineExceeded) || (errors.As(err, &networkError) && networkError.Timeout()) {
			return nil, &ChannelModelsError{Status: http.StatusGatewayTimeout, Code: "upstream_models_timeout"}
		}
		return nil, &ChannelModelsError{Status: http.StatusBadGateway, Code: "upstream_models_network_error"}
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, channelUpstreamError(response, request, key)
	}
	const maxBytes = 2 << 20
	body, err := io.ReadAll(io.LimitReader(response.Body, maxBytes+1))
	if err != nil {
		return nil, &ChannelModelsError{Status: http.StatusBadGateway, Code: "upstream_models_read_failed"}
	}
	if len(body) > maxBytes {
		return nil, &ChannelModelsError{Status: http.StatusBadGateway, Code: "upstream_models_too_large"}
	}
	var result struct {
		Data          *[]ChannelModelInfo `json:"data"`
		HasMore       bool                `json:"has_more"`
		NextCursor    string              `json:"next_cursor"`
		NextPageToken string              `json:"next_page_token"`
		Next          string              `json:"next"`
	}
	if err := json.Unmarshal(body, &result); err != nil || result.Data == nil {
		return nil, &ChannelModelsError{Status: http.StatusBadGateway, Code: "upstream_models_invalid_response"}
	}
	if result.HasMore || result.NextCursor != "" || result.NextPageToken != "" || result.Next != "" {
		return nil, &ChannelModelsError{Status: http.StatusBadGateway, Code: "upstream_models_pagination_unsupported"}
	}
	if len(*result.Data) > 5000 {
		return nil, &ChannelModelsError{Status: http.StatusBadGateway, Code: "upstream_models_too_large"}
	}
	models := make([]ChannelModelInfo, 0, len(*result.Data))
	seen := make(map[string]bool)
	for _, item := range *result.Data {
		id := strings.TrimSpace(item.ID)
		if id == "" {
			continue
		}
		if len(id) > 128 || strings.Contains(id, ",") || strings.ContainsFunc(id, unicode.IsControl) {
			return nil, &ChannelModelsError{Status: http.StatusBadGateway, Code: "upstream_models_invalid_id"}
		}
		if !seen[id] {
			switch item.Domain {
			case "LLM", "VLM", "ImageGeneration", "VideoGeneration", "3DGeneration", "Audio", "Embedding":
			default:
				item.Domain = ""
			}
			models = append(models, ChannelModelInfo{ID: id, Domain: item.Domain})
			seen[id] = true
		}
	}
	return models, nil
}

// channelUpstreamError exposes controlled upstream diagnostics without raw bodies.
func channelUpstreamError(response *http.Response, request *http.Request, key string) *ChannelModelsError {
	code := "upstream_models_failed"
	switch response.StatusCode {
	case http.StatusUnauthorized:
		code = "upstream_models_authentication_failed"
	case http.StatusForbidden:
		code = "upstream_models_permission_denied"
	case http.StatusNotFound:
		code = "upstream_models_not_found"
	case http.StatusTooManyRequests:
		code = "upstream_models_rate_limited"
	default:
		if response.StatusCode >= 300 && response.StatusCode < 400 {
			code = "upstream_models_redirect_refused"
		}
	}
	diagnostic := &ChannelModelsDiagnostic{UpstreamStatus: response.StatusCode}
	if !strings.Contains(request.URL.Host, key) {
		diagnostic.UpstreamHost = request.URL.Host
	}
	// Only standard paths are safe to echo; custom prefixes may contain secrets.
	if request.URL.Path == "/api/v3/models" || request.URL.Path == "/v1/models" {
		diagnostic.CatalogPath = request.URL.Path
	}
	if response.StatusCode == http.StatusUnauthorized || response.StatusCode == http.StatusForbidden {
		body, _ := io.ReadAll(io.LimitReader(response.Body, 8192))
		var details struct {
			RequestID string `json:"request_id"`
			Error     struct {
				Code string `json:"code"`
			} `json:"error"`
		}
		if json.Unmarshal(body, &details) == nil {
			providerCode, _, _ := strings.Cut(details.Error.Code, ".")
			switch providerCode {
			case "AuthenticationError", "InvalidApiKey", "InvalidAPIKey", "InvalidAccessKey", "AccessDenied", "PermissionDenied", "AccountNotAuthorized":
				diagnostic.ProviderCode = providerCode
			}
		}
		requestID := response.Header.Get("X-Request-Id")
		if requestID == "" {
			requestID = response.Header.Get("X-Tt-Logid")
		}
		if requestID == "" {
			requestID = details.RequestID
		}
		if len(requestID) >= 8 && len(requestID) <= 128 && !strings.Contains(requestID, key) && !strings.ContainsFunc(requestID, func(c rune) bool {
			return !((c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '-' || c == '_')
		}) {
			diagnostic.RequestID = requestID
		}
	}
	return &ChannelModelsError{Status: http.StatusBadGateway, Code: code, Diagnostic: diagnostic}
}
