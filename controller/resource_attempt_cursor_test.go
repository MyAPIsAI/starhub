package controller

import (
	"encoding/json"
	"fmt"
	"starhub/model"
	"testing"
	"time"
)

func TestResourceAttemptCursorPagesAndRejectsInvalidBounds(t *testing.T) {
	newTestDB(t)
	at := time.Now().Add(-time.Minute).Unix() / 60 * 60
	for _, id := range []string{"a", "b", "c"} {
		if err := model.DB.Create(&model.ProductionAttemptRecord{ID: id, Minute: at, Payload: `{"attempt_id":"` + id + `"}`}).Error; err != nil {
			t.Fatal(err)
		}
	}
	base := fmt.Sprintf("/api/metric/attempts?start_timestamp=%d&end_timestamp=%d&page_size=2", at, at)
	var data struct {
		Next  string `json:"next_cursor"`
		Items []struct {
			ID string `json:"attempt_id"`
		} `json:"items"`
	}
	env := expectStatus(t, call(ResourcePlatformMetrics, testRequest{target: base}), 200)
	if err := json.Unmarshal(env.Data, &data); err != nil || len(data.Items) != 2 || data.Next == "" {
		t.Fatal(string(env.Data), err)
	}
	env = expectStatus(t, call(ResourcePlatformMetrics, testRequest{target: base + "&cursor=" + data.Next}), 200)
	if err := json.Unmarshal(env.Data, &data); err != nil || len(data.Items) != 1 || data.Items[0].ID != "c" || data.Next != "" {
		t.Fatal(string(env.Data), err)
	}
	for _, extra := range []string{"&cursor=bad", "&cursor=bad&page=2", "&page=1000000"} {
		expectStatus(t, call(ResourcePlatformMetrics, testRequest{target: base + extra}), 400)
	}
}
