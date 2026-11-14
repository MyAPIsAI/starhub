package service

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
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

func TestProductionDeliveryDrainsBacklogWithoutConcurrentDuplicateBatches(t *testing.T) {
	resourceTestDB(t)
	rows := make([]model.ProductionAttemptRecord, 256)
	for i := range rows {
		id := uuid.NewString()
		rows[i] = model.ProductionAttemptRecord{ID: id, Minute: time.Now().Unix() / 60 * 60, Payload: `{"attempt_id":"` + id + `"}`}
	}
	if err := model.DB.CreateInBatches(&rows, 64).Error; err != nil {
		t.Fatal(err)
	}
	seen := map[string]bool{}
	var mu sync.Mutex
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var batch struct {
			Attempts []struct {
				ID string `json:"attempt_id"`
			} `json:"attempts"`
		}
		if err := json.NewDecoder(r.Body).Decode(&batch); err != nil {
			t.Error(err)
		}
		ids := []string{}
		mu.Lock()
		for _, a := range batch.Attempts {
			if seen[a.ID] {
				t.Error("duplicate concurrent delivery")
			}
			seen[a.ID] = true
			ids = append(ids, a.ID)
		}
		mu.Unlock()
		json.NewEncoder(w).Encode(map[string]any{"accepted_ids": ids})
	}))
	defer server.Close()
	t.Setenv("RESOURCE_CATALOG_NOTIFY_URL", server.URL)
	var wg sync.WaitGroup
	failures := make(chan error, 4)
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); failures <- DeliverProductionAttemptsOnce(t.Context()) }()
	}
	wg.Wait()
	close(failures)
	for err := range failures {
		if err != nil {
			t.Fatal(err)
		}
	}
	var pending int64
	model.DB.Model(&model.ProductionAttemptRecord{}).Where("delivered_at = 0").Count(&pending)
	if pending != 0 || len(seen) != 256 {
		t.Fatalf("remaining %d delivered %d", pending, len(seen))
	}
}
