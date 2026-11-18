package service

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"starhub/model"
	"starhub/relay/channel"
)

func TestImageProbePreviewURL(t *testing.T) {
	for _, tc := range []struct {
		name, body, want string
		ok               bool
	}{
		{"signed URL", `{"data":[{"url":"https://images.example/a.png?signature=fixture&expires=123"}]}`, "https://images.example/a.png?signature=fixture&expires=123", true},
		{"local image", `{"data":[{"url":"http://127.0.0.1/image.png"}]}`, "http://127.0.0.1/image.png", true},
		{"first usable", `{"data":[{"url":"javascript:alert(1)"},{"url":"https://images.example/b.png"}]}`, "https://images.example/b.png", true},
		{"base64 only", `{"data":[{"b64_json":"fixture"}]}`, "", true},
		{"credentials", `{"data":[{"url":"https://user:secret@images.example/a.png"}]}`, "", true},
		{"data URL", `{"data":[{"url":"data:image/png;base64,fixture"}]}`, "", true},
		{"relative", `{"data":[{"url":"//images.example/a.png"}]}`, "", true},
		{"oversized URL", fmt.Sprintf(`{"data":[{"url":%q}]}`, "https://images.example/"+strings.Repeat("a", 8192)), "", true},
		{"invalid response", `{"data":[{"url":"https://images.example/a.png"},{"url":42}]}`, "", false},
		{"no payload", `{"data":[]}`, "", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			hits := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				hits++
				if r.Method != "POST" || r.URL.Path != "/api/v3/images/generations" {
					t.Error("unexpected request")
				}
				fmt.Fprint(w, tc.body)
			}))
			defer server.Close()
			ch := &model.Channel{Type: channel.ChannelTypeArk, BaseURL: server.URL, Key: "preview-secret-key"}
			got := ProbeChannelImage(t.Context(), ch, "fixture-image")
			if got.OK != tc.ok || got.ImageURL != tc.want || hits != 1 {
				t.Fatalf("ok=%v, preview=%q, requests=%d", got.OK, got.ImageURL, hits)
			}
		})
	}
}
