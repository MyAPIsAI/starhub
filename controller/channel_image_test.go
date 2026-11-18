package controller

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"starhub/model"
	"starhub/service"
)

func TestChannelImageResultPersistsAndScopesToConnection(t *testing.T) {
	newTestDB(t)
	calls := 0
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		fmt.Fprint(w, `{"data":[{"url":"https://example.test/probe.png?signature=signed-fixture"}]}`)
	}))
	defer upstream.Close()
	ch := &model.Channel{Name: "image-result", Type: 32, BaseURL: upstream.URL, Key: "image-secret-key", Models: "image-model", Status: 1}
	if err := model.InsertChannel(ch); err != nil {
		t.Fatal(err)
	}
	get := func() *httptest.ResponseRecorder {
		return call(GetChannelImageTest, testRequest{method: "GET", target: fmt.Sprintf("/api/channel/image-test/%d/result", ch.ID), path: map[string]string{"id": fmt.Sprint(ch.ID)}})
	}
	var empty struct {
		Data *channelTestResult `json:"data"`
	}
	if err := json.Unmarshal(get().Body.Bytes(), &empty); err != nil {
		t.Fatal(err)
	}
	if empty.Data != nil || calls != 0 {
		t.Fatal("viewing an empty result submitted an image")
	}
	generated := service.ProbeChannelImage(t.Context(), ch, "image-model")
	if !generated.OK {
		t.Fatal("fixture image did not generate")
	}
	for range 2 {
		var restored channelTestResult
		response := get()
		decodeData(t, response, &restored)
		if !restored.OK || restored.ImageURL != generated.ImageURL || restored.Model != "image-model" || restored.Scope != "image" || calls != 1 {
			t.Fatalf("image result did not survive reload: %+v", restored)
		}
		if strings.Contains(response.Body.String(), ch.Key) || strings.Contains(response.Body.String(), "connection_hash") {
			t.Fatal("private channel configuration was exposed")
		}
	}
	ch.Key = "different-image-key"
	if err := model.UpdateChannel(ch); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(get().Body.Bytes(), &empty); err != nil {
		t.Fatal(err)
	}
	if empty.Data != nil || calls != 1 {
		t.Fatal("a different connection exposed the old preview")
	}
}
