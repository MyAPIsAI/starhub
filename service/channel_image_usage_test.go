package service

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"starhub/model"
)

func TestImageProbePersistsUsageBeforeHTTP(t *testing.T) {
	newTestDB(t)
	status, calls := 200, 0
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		var recorded int64
		model.DB.Model(&model.ChannelImageProbe{}).Where("channel_id = ? AND state = ?", 1, "uncertain").Count(&recorded)
		if recorded == 0 {
			t.Error("paid image request was not recorded before HTTP")
		}
		w.WriteHeader(status)
		fmt.Fprint(w, `{"data":[{"url":"https://example.test/image.png","b64_json":"fixture"}]}`)
	}))
	defer upstream.Close()
	ch := &model.Channel{ID: 1, Type: 32, BaseURL: upstream.URL, Key: "fixture-key"}
	for _, code := range []int{200, 401, 500} {
		status = code
		result := ProbeChannelImage(t.Context(), ch, "image-model")
		if result.OK != (code == 200) {
			t.Fatal("probe behavior changed")
		}
	}
	usage, err := model.GetChannelProbeUsage(t.Context(), ch.ID)
	if err != nil || usage.Images != 1 || usage.ImageTests != 3 || usage.ImageSuccesses != 1 || usage.UncertainImages != 1 {
		t.Fatalf("unexpected image usage: %+v, %v", usage, err)
	}
	if err := model.DB.Migrator().DropTable(&model.ChannelImageProbe{}); err != nil {
		t.Fatal(err)
	}
	blocked := ProbeChannelImage(t.Context(), ch, "image-model")
	if blocked.OK || blocked.Code != "image_probe_usage_unavailable" || calls != 3 {
		t.Fatal("image generation continued without durable local recording")
	}
}
