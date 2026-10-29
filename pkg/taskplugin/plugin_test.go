package taskplugin

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"
)

func example(t *testing.T) string {
	t.Helper()
	source, err := os.ReadFile("../../plugins/tasks/openai-video.js")
	if err != nil {
		t.Fatal(err)
	}
	return string(source)
}

func TestPluginIsolationAndTimeout(t *testing.T) {
	source := example(t)
	for _, change := range []string{
		"\nwhile(true) {}",
		"\nprepareSubmit = function(){ while(true) {} };",
		"\nprepareSubmit = function(){ return {toJSON:function(){while(true){}}}; };",
	} {
		p, err := Compile(context.Background(), source+change)
		if err == nil {
			var out any
			err = p.Call(context.Background(), "prepareSubmit", []any{map[string]any{}}, &out)
		}
		if err == nil {
			t.Fatal("unbounded execution was accepted")
		}
	}
	p, err := Compile(context.Background(), source+`var calls=0; prepareSubmit=function(){return {calls:++calls, network:typeof fetch, files:typeof require};};`)
	if err != nil {
		t.Fatal(err)
	}
	for range 2 {
		var out struct {
			Calls   int    `json:"calls"`
			Network string `json:"network"`
			Files   string `json:"files"`
		}
		if err := p.Call(context.Background(), "prepareSubmit", nil, &out); err != nil {
			t.Fatal(err)
		}
		if out.Calls != 1 || out.Network != "undefined" || out.Files != "undefined" {
			t.Fatalf("runtime isolation lost: %+v", out)
		}
	}
	p, err = Compile(context.Background(), source+`parseSubmit=function(){throw new Error("credential-like-content");};`)
	if err != nil {
		t.Fatal(err)
	}
	var out any
	if err := p.Call(context.Background(), "parseSubmit", nil, &out); err == nil || strings.Contains(err.Error(), "credential-like-content") {
		t.Fatalf("script exception leaked: %v", err)
	}
}

func TestPluginValidationAndHTTPBoundary(t *testing.T) {
	for _, source := range []string{example(t) + `meta.apiVersion=2;`, example(t) + `parseSubmit=1;`, example(t) + `meta.auth="custom";`, strings.Repeat("x", MaxSource+1)} {
		if _, err := Compile(context.Background(), source); err == nil {
			t.Fatal("invalid plugin accepted")
		}
	}
	for _, path := range []string{"https://other.test/tasks", "//other.test/tasks", "/../tasks", "/%2e%2e/tasks", "/tasks#secret", "/\\other/tasks"} {
		if _, err := (Request{Method: "GET", Path: path}).URL("https://upstream.test/base", "GET"); err == nil {
			t.Fatalf("unsafe path accepted: %s", path)
		}
	}
	if _, err := (Request{Method: "GET", Path: "/tasks", Headers: map[string]string{"Authorization": "override"}}).URL("https://upstream.test", "GET"); err == nil {
		t.Fatal("credential override accepted")
	}
	var redirected atomic.Int32
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { redirected.Add(1) }))
	defer target.Close()
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer host-secret" {
			t.Error("host authentication missing")
		}
		if r.URL.Path == "/redirect" {
			http.Redirect(w, r, target.URL, http.StatusTemporaryRedirect)
			return
		}
		_, _ = w.Write([]byte(`{"id":"ok"}`))
	}))
	defer upstream.Close()
	p, err := Compile(context.Background(), example(t))
	if err != nil {
		t.Fatal(err)
	}
	response, err := p.Do(context.Background(), upstream.URL, "host-secret", Request{Method: "POST", Path: "/tasks", Body: json.RawMessage(`{}`)}, "POST")
	if err != nil || response.Status != 200 {
		t.Fatalf("host request failed: %+v %v", response, err)
	}
	_, _ = p.Do(context.Background(), upstream.URL, "host-secret", Request{Method: "POST", Path: "/redirect", Body: json.RawMessage(`{}`)}, "POST")
	if redirected.Load() != 0 {
		t.Fatal("redirect followed with credentials")
	}
}

func TestPluginJSONSizeLimits(t *testing.T) {
	p, err := Compile(context.Background(), example(t)+`prepareSubmit=function(){return "x".repeat(1048577);};`)
	if err != nil {
		t.Fatal(err)
	}
	var out any
	if err := p.Call(context.Background(), "prepareSubmit", nil, &out); err == nil {
		t.Fatal("oversized output accepted")
	}
	if err := p.Call(context.Background(), "parseSubmit", []any{strings.Repeat("x", MaxJSON+1)}, &out); err == nil {
		t.Fatal("oversized input accepted")
	}
}

func TestPluginRequestPreservesEscapedPath(t *testing.T) {
	for _, tc := range []struct{ base, path, want string }{
		{"https://vendor.test", "/videos/job%2Fpart", "https://vendor.test/videos/job%2Fpart"},
		{"https://vendor.test/base%2Ftenant", "/videos/job%252Fpart?detail=1", "https://vendor.test/base%2Ftenant/videos/job%252Fpart?detail=1"},
		{"https://vendor.test/base/", "/videos/job%3Fpart%23name", "https://vendor.test/base/videos/job%3Fpart%23name"},
	} {
		got, err := (Request{Method: "GET", Path: tc.path}).URL(tc.base, "GET")
		if err != nil || got != tc.want {
			t.Fatalf("URL = %q, err = %v, want %q", got, err, tc.want)
		}
	}
	seen := make(chan string, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { seen <- r.RequestURI; _, _ = w.Write([]byte(`{}`)) }))
	defer server.Close()
	p, err := Compile(context.Background(), example(t))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := p.Do(context.Background(), server.URL+"/base", "key", Request{Method: "GET", Path: "/videos/job%2Fpart"}, "GET"); err != nil {
		t.Fatal(err)
	}
	if got := <-seen; got != "/base/videos/job%2Fpart" {
		t.Fatalf("wire request lost task ID encoding: %s", got)
	}
}
