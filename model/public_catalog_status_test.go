package model

import (
	"testing"
	"time"

	"starhub/setting"
)

func TestPublicCatalogStatusCustomerDenominatorAndLatency(t *testing.T) {
	groupConfigurationDB(t)
	if _, err := SaveOption(setting.OptionKeyGroupRatio, `{"high":1,"second":0.8,"low":0.1,"private":2}`, 0); err != nil {
		t.Fatal(err)
	}
	target := int64(500)
	for _, id := range []string{"high", "second", "low", "private"} {
		visibility := "common"
		if id == "private" {
			visibility = "custom"
		}
		if err := UpsertServiceGroup(&ServiceGroupConfig{GroupID: id, Name: id, Enabled: true, Visibility: visibility, SLAP95MS: &target}); err != nil {
			t.Fatal(err)
		}
		if err := DB.Model(&ServiceGroupConfig{}).Where("group_id = ?", id).Update("sla_p95_ms", target).Error; err != nil {
			t.Fatal(err)
		}
	}
	at := time.Now()
	insert := func(group string, status int, code string, count int, ttft, prompt int64) {
		t.Helper()
		for i := 0; i < count; i++ {
			if err := DB.Create(&Log{Model: "status-chat", RequestedModel: "veyra-auto", ServiceGroup: group, Status: status, ErrorCode: code, TTFTMs: ttft, PromptTokens: prompt, CreatedAt: at.Unix()}).Error; err != nil {
				t.Fatal(err)
			}
		}
	}
	read := func() CatalogStatus {
		t.Helper()
		s, e := PublicCatalogStatus(t.Context(), "status-chat", []string{"text"}, at)
		if e != nil {
			t.Fatal(e)
		}
		return s
	}
	insert("high", 1, "", 49, 100, 100)
	insert("high", 3, "request_cancelled", 20, 0, 0)
	insert("high", 2, "upstream_client_error", 20, 0, 0)
	insert("private", 2, "upstream_unavailable", 50, 0, 0)
	insert("low", 2, "upstream_unavailable", 50, 0, 0)
	if s := read(); s.State != "insufficient_data" || s.Samples != 49 || s.SuccessRate != nil {
		t.Fatal(s)
	}
	insert("second", 1, "", 1, 100, 100)
	if s := read(); s.State != "normal" || s.Samples != 50 || s.TTFTP95MS == nil || *s.TTFTP95MS != 100 {
		t.Fatal(s)
	}
	insert("high", 1, "", 5, 900, 100)
	insert("high", 1, "", 100, 9000, 3000)
	if s := read(); s.State != "degraded" || s.TTFTP95MS == nil || *s.TTFTP95MS != 900 {
		t.Fatal(s)
	}
	insert("second", 2, "upstream_unavailable", 20, 0, 0)
	if s := read(); s.State != "outage" {
		t.Fatal(s)
	}
	s, e := PublicCatalogStatus(t.Context(), "veyra-auto", []string{"text"}, at)
	if e != nil || s.State != "outage" {
		t.Fatal(s, e)
	}
	s, e = PublicCatalogStatus(t.Context(), "status-chat", []string{"audio"}, at)
	if e != nil || s.TTFTP95MS != nil {
		t.Fatal(s, e)
	}
}
