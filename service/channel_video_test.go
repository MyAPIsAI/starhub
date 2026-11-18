package service

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"starhub/model"
)

func TestBytePlusVideoConnectivityWithoutCapabilities(t *testing.T) {
	reads := 0
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" || r.URL.Path != "/api/v3/contents/generations/tasks" || r.Header.Get("Authorization") != "Bearer fixture" {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
			w.WriteHeader(500)
			return
		}
		reads++
		fmt.Fprint(w, `{"items":[],"total":0}`)
	}))
	defer upstream.Close()
	for _, name := range []string{"dreamina-seedance-2-0-fast-260128", "seedance-1-0-pro-250528"} {
		ch := &model.Channel{Type: 32, BaseURL: upstream.URL, Key: "fixture", Models: name}
		result := ProbeChannel(t.Context(), ch, name)
		if !result.OK || result.Scope != "video_api" {
			t.Fatalf("video connectivity used the catalog: %+v", result)
		}
		image := ProbeChannelImage(t.Context(), ch, name)
		if image.Code != "image_probe_model_not_supported" {
			t.Fatal("video model admitted to image generation")
		}
	}
	if reads != 2 {
		t.Fatal("unexpected upstream request count")
	}
}
