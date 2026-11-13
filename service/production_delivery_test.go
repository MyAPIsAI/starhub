package service

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
	"starhub/model"
)

func TestProductionAttemptDeliveryRetriesStableIDsAndRequiresExactReceipt(t *testing.T) {
	resourceTestDB(t)
	id := uuid.NewString()
	row := model.ProductionAttemptRecord{ID: id, Minute: time.Now().Unix() / 60 * 60, Payload: `{"attempt_id":"` + id + `"}`}
	if err := model.DB.Create(&row).Error; err != nil {
		t.Fatal(err)
	}
	t.Setenv("NODE_NAME", "node-test")
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.URL.Path != "/internal/routing/attempt-metadata" || r.Header.Get("X-Protection-Node") != "node-test" {
			t.Error("wrong delivery identity")
		}
		var batch struct {
			Attempts []struct {
				ID string `json:"attempt_id"`
			} `json:"attempts"`
		}
		if json.NewDecoder(r.Body).Decode(&batch) != nil || len(batch.Attempts) != 1 || batch.Attempts[0].ID != id {
			t.Error("attempt identity changed")
		}
		w.Header().Set("Content-Type", "application/json")
		if calls == 1 {
			fmt.Fprint(w, `{"accepted_ids":["wrong"]}`)
		} else {
			fmt.Fprintf(w, `{"accepted_ids":[%q]}`, id)
		}
	}))
	defer server.Close()
	t.Setenv("RESOURCE_CATALOG_NOTIFY_URL", server.URL)
	if err := DeliverProductionAttemptsOnce(t.Context()); err == nil {
		t.Fatal("foreign acknowledgement accepted")
	}
	model.DB.First(&row, "id = ?", id)
	if row.DeliveredAt != 0 || row.Attempts != 1 || row.NextAttemptAt <= time.Now().Unix() {
		t.Fatal("failed delivery not retained for retry")
	}
	model.DB.Model(&row).Update("next_attempt_at", 0)
	if err := DeliverProductionAttemptsOnce(t.Context()); err != nil {
		t.Fatal(err)
	}
	if err := DeliverProductionAttemptsOnce(t.Context()); err != nil {
		t.Fatal(err)
	}
	if calls != 2 {
		t.Fatal("acknowledged attempt resent")
	}
}
