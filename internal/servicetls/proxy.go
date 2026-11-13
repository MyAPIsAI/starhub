package servicetls

import (
	"errors"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"path"
	"strconv"
	"strings"
)

func ValidateEndpoint(direction, listen string, target *url.URL) error {
	if target == nil || target.Hostname() == "" || target.User != nil || target.RawQuery != "" || target.Fragment != "" || (target.Path != "" && target.Path != "/") || target.RawPath != "" {
		return errors.New("upstream must be a base URL without credentials, query, fragment or path")
	}
	host, port, err := net.SplitHostPort(listen)
	if err != nil {
		return errors.New("listen must include an explicit host and port")
	}
	number, err := strconv.Atoi(port)
	if err != nil || number < 1 || number > 65535 {
		return errors.New("invalid listen port")
	}
	if direction == "ingress" {
		ip := net.ParseIP(target.Hostname())
		if target.Scheme != "http" || ip == nil || !ip.IsLoopback() {
			return errors.New("ingress upstream must be HTTP on an explicit loopback address")
		}
	} else if direction == "egress" {
		ip := net.ParseIP(host)
		if target.Scheme != "https" || ip == nil || !ip.IsLoopback() {
			return errors.New("egress must listen on loopback and forward only to HTTPS")
		}
	} else {
		return errors.New("direction must be ingress or egress")
	}
	return nil
}

func allowed(role, method, route string) bool {
	if route != path.Clean(route) {
		return false
	}
	if role == "gateway" {
		if method == http.MethodPost {
			return route == "/api/metric/runtime/summary"
		}
		if method != http.MethodGet {
			return false
		}
		switch route {
		case "/api/metric/redis", "/api/metric/mysql", "/api/metric/model/redis", "/api/metric/model/mysql", "/api/metric/attempts", "/api/metric/routing/policy-status", "/api/channel/models/details", "/api/channel/models/changes", "/api/channel/catalog/version":
			return true
		}
		if strings.HasPrefix(route, "/api/channel/test/") {
			id, err := strconv.ParseInt(strings.TrimPrefix(route, "/api/channel/test/"), 10, 64)
			return err == nil && id > 0
		}
	} else if role == "resource" && method == http.MethodPost {
		switch route {
		case "/internal/catalog/events", "/internal/routing/protection-events", "/internal/routing/hard-failure-events", "/internal/routing/observations/v2/claim", "/internal/routing/observations/v2/results", "/internal/routing/attempt-metadata":
			return true
		}
	}
	return false
}

func Proxy(target *url.URL, role string, transport http.RoundTripper) (http.Handler, error) {
	if role != "gateway" && role != "resource" {
		return nil, errors.New("role must be gateway or resource")
	}
	if target == nil {
		return nil, errors.New("upstream is required")
	}
	proxy := &httputil.ReverseProxy{Transport: transport, Rewrite: func(request *httputil.ProxyRequest) {
		request.SetURL(target)
		request.Out.Host = target.Host
		request.Out.Header.Del("Forwarded")
		request.Out.Header.Del("X-Forwarded-For")
		request.Out.Header.Del("X-Forwarded-Host")
		request.Out.Header.Del("X-Forwarded-Proto")
		request.SetXForwarded()
	}, ModifyResponse: func(response *http.Response) error {
		if response.StatusCode >= 300 && response.StatusCode < 400 {
			return errors.New("service redirects are not allowed")
		}
		return nil
	}, ErrorHandler: func(w http.ResponseWriter, r *http.Request, err error) {
		http.Error(w, "service transport unavailable", http.StatusBadGateway)
	}}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.RawPath != "" || !allowed(role, r.Method, r.URL.Path) {
			http.NotFound(w, r)
			return
		}
		proxy.ServeHTTP(w, r)
	}), nil
}
