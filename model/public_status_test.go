package model

import (
	"starhub/setting"
	"testing"
	"time"
)

func statusFixture(t *testing.T) time.Time {
	t.Helper()
	groupConfigurationDB(t)
	if _, err := SaveOption(setting.OptionKeyGroupRatio, `{"highest":1,"second":0.8,"low":0.1,"private":2}`, 0); err != nil {
		t.Fatal(err)
	}
	for i, id := range []string{"highest", "second", "low", "private"} {
		visibility := "common"
		if id == "private" {
			visibility = "custom"
		}
		g := ServiceGroupConfig{GroupID: id, Name: id, Visibility: visibility, Enabled: true}
		if err := UpsertServiceGroup(&g); err != nil {
			t.Fatal(err)
		}
		if err := DB.Model(&ServiceGroupConfig{}).Where("id = ?", g.ID).Update("sla_p95_ms", 500+int64(i)*500).Error; err != nil {
			t.Fatal(err)
		}
	}
	at, _ := time.Parse(time.RFC3339, "2026-11-09T12:00:00+04:00")
	return at
}
func statusCalls(t *testing.T, at time.Time, name, group string, successful, failed int, code string, ttft, tokens int64) {
	t.Helper()
	rows := []Log{}
	for i := 0; i < successful+failed; i++ {
		status := 1
		if i >= successful {
			status = 2
		}
		rows = append(rows, Log{Model: name, ServiceGroup: group, CreatedAt: at.Unix(), Status: status, ErrorCode: code, TTFTMs: ttft, PromptTokens: tokens, CallType: "chat"})
	}
	if len(rows) > 0 {
		if err := DB.CreateInBatches(&rows, 100).Error; err != nil {
			t.Fatal(err)
		}
	}
}
func statusRead(t *testing.T, at time.Time, zone string, names map[string]bool) map[string]PublicModelStatus {
	t.Helper()
	c, err := ReadServiceGroupCatalog(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	s, err := PublicStatusHistory(t.Context(), names, StatusPageGroups(c), zone, at)
	if err != nil {
		t.Fatal(err)
	}
	return s
}
func TestUIAC04PublicStatusOutageIgnoresLowGroups(t *testing.T) {
	at := statusFixture(t)
	statusCalls(t, at, "outage", "highest", 45, 5, "upstream_unavailable", 100, 100)
	statusCalls(t, at, "outage", "low", 500, 0, "", 100, 100)
	for _, code := range []string{"CLIENT_ERROR", "CREDENTIAL_INVALID", "MODEL_UNAVAILABLE", "CONTENT_FILTERED", "upstream_client_error", "request_cancelled", "historical_unknown"} {
		statusCalls(t, at, "outage", "highest", 0, 30, code, 100, 100)
	}
	s := statusRead(t, at, "Asia/Dubai", map[string]bool{"outage": true})["outage"]
	if s.Current.Samples != 50 || s.Current.State != "outage" || s.Current.SuccessRate == nil || *s.Current.SuccessRate != 0.9 || s.Daily[89].State != "outage" {
		t.Fatal(s)
	}
	statusCalls(t, at, "zero", "highest", 0, 50, "upstream_unavailable", 0, 0)
	z := statusRead(t, at, "Asia/Dubai", map[string]bool{"zero": true})["zero"]
	if z.Current.State != "outage" || z.Last24Hours.SuccessRate == nil || *z.Last24Hours.SuccessRate != 0 {
		t.Fatal(z)
	}
}
func TestUIAC17PublicStatusInsufficientSampleDenominator(t *testing.T) {
	at := statusFixture(t)
	statusCalls(t, at, "few", "highest", 40, 0, "", 100, 100)
	statusCalls(t, at, "few", "low", 500, 0, "", 100, 100)
	statusCalls(t, at, "few", "highest", 0, 30, "upstream_client_error", 100, 100)
	statusCalls(t, at, "few", "private", 100, 0, "", 100, 100)
	s := statusRead(t, at, "Asia/Dubai", map[string]bool{"few": true})["few"]
	if s.Current.Samples != 40 || s.Current.State != "insufficient_data" || s.Current.SuccessRate != nil || s.Daily[89].State != "insufficient_data" {
		t.Fatal(s)
	}
	if s.Last24Hours.SuccessRate == nil || *s.Last24Hours.SuccessRate != 1 || s.Last24Hours.Samples != 40 {
		t.Fatal(s)
	}
}
func TestPublicStatusWindowsLatencyAndDailyThresholds(t *testing.T) {
	at := statusFixture(t)
	statusCalls(t, at, "chat", "highest", 47, 0, "", 100, 2000)
	statusCalls(t, at, "chat", "second", 3, 0, "", 1500, 2000)
	statusCalls(t, at, "chat", "highest", 50, 0, "", 99999, 2001)
	statusCalls(t, at.Add(-2*time.Hour), "chat", "highest", 50, 0, "", 700, 100)
	statusCalls(t, at.Add(-20*time.Minute), "chat", "highest", 0, 100, "upstream_unavailable", 0, 0)
	statusCalls(t, at, "audio", "highest", 50, 0, "", 5000, 1)
	statusCalls(t, at.Add(time.Second), "chat", "highest", 0, 100, "upstream_unavailable", 0, 0)
	for i, c := range []struct{ success, failure int }{{999, 1}, {995, 5}, {980, 20}, {49, 0}} {
		statusCalls(t, at.AddDate(0, 0, -i-1), "daily", "highest", c.success, c.failure, "upstream_unavailable", 100, 100)
	}
	s := statusRead(t, at, "Asia/Dubai", map[string]bool{"chat": true, "audio": false, "daily": true, "empty": true})
	c := s["chat"]
	if c.Current.State != "degraded" || c.Current.Samples != 100 || *c.Current.TTFTP95MS != 1500 {
		t.Fatal(c.Current)
	}
	if c.Last24Hours.Samples != 250 || c.Last24Hours.TTFTSamples != 100 || *c.Last24Hours.SuccessRate != 0.6 || *c.Last24Hours.TTFTP50MS != 700 || *c.Last24Hours.TTFTP95MS != 700 {
		t.Fatal(c.Last24Hours)
	}
	if s["audio"].Current.State != "normal" || s["audio"].Last24Hours.TTFTP95MS != nil {
		t.Fatal(s["audio"])
	}
	for i, want := range []string{"normal", "degraded", "outage", "insufficient_data"} {
		if s["daily"].Daily[88-i].State != want {
			t.Fatal(s["daily"].Daily[88-i])
		}
	}
	if len(s["empty"].Daily) != 90 || s["empty"].Last24Hours.SuccessRate != nil || s["empty"].Current.State != "insufficient_data" {
		t.Fatal(s["empty"])
	}
}
func TestPublicStatusRegionalDaysAcrossDST(t *testing.T) {
	statusFixture(t)
	at, _ := time.Parse(time.RFC3339, "2026-11-02T00:30:00-05:00")
	for _, stamp := range []string{"2026-11-01T00:00:00-04:00", "2026-11-01T23:59:59-05:00"} {
		when, _ := time.Parse(time.RFC3339, stamp)
		statusCalls(t, when, "dst", "highest", 25, 0, "", 100, 100)
	}
	statusCalls(t, at, "dst", "highest", 49, 0, "", 100, 100)
	s := statusRead(t, at, "America/New_York", map[string]bool{"dst": true})["dst"]
	if s.Daily[88].Date != "2026-11-01" || s.Daily[88].Samples != 50 || s.Daily[88].State != "normal" || s.Daily[89].Samples != 49 {
		t.Fatal(s.Daily[88:])
	}
}
func TestPublicStatusGroupOverridesAndLegacyPreservation(t *testing.T) {
	at := statusFixture(t)
	statusCalls(t, at, "scope", "low", 50, 0, "", 100, 100)
	for id, mode := range map[string]string{"highest": "exclude", "second": "exclude", "low": "include"} {
		g, _ := GetServiceGroup(id)
		g.StatusPageMode = mode
		if err := UpsertServiceGroup(g); err != nil {
			t.Fatal(err)
		}
	}
	legacy := ServiceGroupConfig{GroupID: "low", Name: "Renamed group", Enabled: true, Visibility: "common"}
	if err := UpsertServiceGroup(&legacy); err != nil {
		t.Fatal(err)
	}
	if legacy.StatusPageMode != "include" {
		t.Fatal("legacy edit reset status selection")
	}
	s := statusRead(t, at, "Asia/Dubai", map[string]bool{"scope": true})["scope"]
	if s.Current.State != "normal" || s.Current.Samples != 50 {
		t.Fatal(s)
	}
	legacy.Enabled = false
	if err := UpsertServiceGroup(&legacy); err != nil {
		t.Fatal(err)
	}
	if s := statusRead(t, at, "Asia/Dubai", map[string]bool{"scope": true})["scope"]; s.Current.State != "insufficient_data" {
		t.Fatal(s)
	}
	legacy.StatusPageMode = "invalid"
	if err := UpsertServiceGroup(&legacy); err != ErrInvalidStatusPageMode {
		t.Fatal(err)
	}
}

func TestPublicStatusLegacyGroupsAndDeletedDefinitions(t *testing.T) {
	groupConfigurationDB(t)
	if _, err := SaveOption(setting.OptionKeyGroupRatio, `{"legacy-high":1,"legacy-low":0.5,"deleted":5,"disabled":6,"custom":7}`, 0); err != nil {
		t.Fatal(err)
	}
	for _, g := range []ServiceGroupConfig{{GroupID: "disabled", Visibility: "common", Enabled: false}, {GroupID: "custom", Visibility: "custom", Enabled: true}} {
		if err := UpsertServiceGroup(&g); err != nil {
			t.Fatal(err)
		}
	}
	if err := DB.Create(&ServiceGroupDeletion{GroupID: "deleted"}).Error; err != nil {
		t.Fatal(err)
	}
	catalog, err := ReadServiceGroupCatalog(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	groups := StatusPageGroups(catalog)
	if len(groups) != 2 || groups[0].GroupID != "legacy-high" || groups[1].GroupID != "legacy-low" {
		t.Fatal(groups)
	}
	if groups := PublicCommonGroups(catalog); len(groups) != 2 {
		t.Fatal(groups)
	}
	at := time.Now()
	statusCalls(t, at, "legacy-model", "legacy-high", 45, 5, "CONNECT_FAIL", 100, 100)
	data, err := PublicStatusHistory(t.Context(), map[string]bool{"legacy-model": true}, groups, "Asia/Dubai", at)
	if err != nil || data["legacy-model"].Current.State != "outage" {
		t.Fatal(data, err)
	}
}
