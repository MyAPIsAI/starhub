package main

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"starhub/common"
	"starhub/middleware"
	"starhub/model"
	"starhub/relay"
)

func TestSavedZeroCacheRatioRemainsZeroInActualPlazaQuote(t *testing.T) {
	env := setupRelayApp(t)
	if err := model.InsertChannel(&model.Channel{Name: "Zero cache quote fixture", Type: 1, Models: "gpt-4o", Group: "default", Status: common.ChannelStatusEnabled}); err != nil {
		t.Fatal(err)
	}
	response := doRequest(env.base, "PUT", "/api/option/pricing", env.session, map[string]any{"expected_version": 0, "options": []model.Option{{Key: "cache_ratio", Value: "0"}, {Key: "audio_ratio", Value: "0"}}})
	if response.Status != 200 {
		t.Fatalf("save zero details: %d %s", response.Status, response.Body)
	}
	response = doRequest(env.base, "GET", "/api/plaza/models/gpt-4o", env.session, nil)
	var detail struct {
		Data struct {
			Model struct {
				Pricing struct {
					CacheRead *float64 `json:"cache_read_per_m"`
					Input     *float64 `json:"input_per_m"`
				} `json:"pricing"`
			} `json:"model"`
		} `json:"data"`
	}
	if err := json.Unmarshal([]byte(response.Body), &detail); err != nil || response.Status != 200 || detail.Data.Model.Pricing.CacheRead == nil || *detail.Data.Model.Pricing.CacheRead != 0 || detail.Data.Model.Pricing.Input == nil || *detail.Data.Model.Pricing.Input <= 0 {
		t.Fatalf("zero cache quote: %d %s %v", response.Status, response.Body, err)
	}
	pd := relay.NewPriceData("gpt-4o", "default")
	if pd.CacheRatio != 0 || pd.AudioRatio != 0 {
		t.Fatal("zero detail multipliers were not loaded")
	}
	quota := relay.ComputeUsageQuota(pd, &relay.Usage{PromptTokens: 1_000_000, PromptTokensDetails: &relay.PromptTokensDetails{CachedTokens: 1_000_000}})
	if quota != 1 {
		t.Fatalf("zero cached unit price must retain token-billing minimum: %d", quota)
	}
	if root, err := model.GetUserByID(env.rootID); err != nil || root.Quota != 100_000_000 {
		t.Fatal("quote or cost projection changed account quota")
	}
}

func TestPricingOptionsRouteGuardsAtomicCommitAndReloadOutcome(t *testing.T) {
	env := setupRelayApp(t)
	if err := model.InsertChannel(&model.Channel{Name: "Pricing quote fixture", Type: 1, Models: "gpt-4o", Group: "default", Status: common.ChannelStatusEnabled}); err != nil {
		t.Fatal(err)
	}
	read := func() struct {
		Version int64          `json:"version"`
		Loaded  int64          `json:"loaded_version"`
		Atomic  bool           `json:"atomic_batch"`
		Options []model.Option `json:"options"`
	} {
		t.Helper()
		response := doRequest(env.base, "GET", "/api/option/pricing", env.session, nil)
		var envelope struct {
			Data struct {
				Version int64          `json:"version"`
				Loaded  int64          `json:"loaded_version"`
				Atomic  bool           `json:"atomic_batch"`
				Options []model.Option `json:"options"`
			} `json:"data"`
		}
		if err := json.Unmarshal([]byte(response.Body), &envelope); err != nil || response.Status != 200 {
			t.Fatalf("pricing read: %d %s %v", response.Status, response.Body, err)
		}
		return envelope.Data
	}
	state := read()
	if !state.Atomic || state.Version != 0 || state.Loaded != 0 || len(state.Options) != 6 {
		t.Fatalf("initial snapshot: %+v", state)
	}
	batch := func(version int64, input, completion string) map[string]any {
		return map[string]any{"expected_version": version, "options": []model.Option{{Key: "model_ratio", Value: `{"gpt-4o":` + input + `,"preserved":0}`}, {Key: "completion_ratio", Value: `{"gpt-4o":` + completion + `,"preserved":0}`}}}
	}
	for _, role := range []int{common.RoleUser, common.RoleFinance, 99, 101, common.RoleAdmin} {
		user := &model.User{Username: fmt.Sprintf("pricing-role-%d", role), Role: role, Status: common.StatusEnabled, AffCode: common.GenerateAffCode()}
		if err := model.CreateUser(user); err != nil {
			t.Fatal(err)
		}
		session := middleware.Sessions.Create(user.ID, time.Hour)
		t.Cleanup(func() { middleware.Sessions.Remove(session) })
		want := 403
		if role == common.RoleAdmin {
			want = 200
		}
		if response := doRequest(env.base, "GET", "/api/option/pricing", session, nil); response.Status != want {
			t.Fatalf("role %d read: %d %s", role, response.Status, response.Body)
		}
		response := doRequest(env.base, "PUT", "/api/option/pricing", session, map[string]any{"expected_version": 0, "options": []model.Option{state.Options[0]}})
		if response.Status != want {
			t.Fatalf("role %d write: %d %s", role, response.Status, response.Body)
		}
	}
	for _, method := range []string{"GET", "PUT"} {
		if response := doRequest(env.base, method, "/api/option/pricing", "", batch(0, "3", "4")); response.Status != 401 {
			t.Fatalf("anonymous %s: %d", method, response.Status)
		}
	}
	if response := doRequest(env.base, "PUT", "/api/option/pricing", env.session, batch(0, "3", "4")); response.Status != 200 {
		t.Fatalf("valid batch: %d %s", response.Status, response.Body)
	}
	state = read()
	if state.Version != 2 || state.Loaded != 2 {
		t.Fatalf("pair did not load as one commit: %+v", state)
	}
	checkQuote := func(input, output float64) {
		t.Helper()
		response := doRequest(env.base, "GET", "/api/plaza/models/gpt-4o", "", nil)
		var detail struct {
			Data struct {
				Model struct {
					Pricing struct {
						Input  float64 `json:"input_per_m"`
						Output float64 `json:"output_per_m"`
					} `json:"pricing"`
				} `json:"model"`
			} `json:"data"`
		}
		if err := json.Unmarshal([]byte(response.Body), &detail); err != nil || response.Status != 200 || detail.Data.Model.Pricing.Input != input || detail.Data.Model.Pricing.Output != output {
			t.Fatalf("quote mismatch: %d %s %v", response.Status, response.Body, err)
		}
	}
	checkQuote(3, 12)
	for _, payload := range []map[string]any{{"options": state.Options}, {"expected_version": nil, "options": state.Options}, {"expected_version": 2.5, "options": state.Options}, {"expected_version": -1, "options": state.Options}, {"expected_version": 2, "options": []model.Option{}}, {"expected_version": 2, "options": []model.Option{{Key: "group_ratio", Value: "{}"}}}, {"expected_version": 2, "options": []model.Option{{Key: "model_ratio", Value: "invalid"}}}, {"expected_version": 2, "options": state.Options, "unknown": true}} {
		if response := doRequest(env.base, "PUT", "/api/option/pricing", env.session, payload); response.Status != 400 {
			t.Fatalf("invalid batch: %d %s", response.Status, response.Body)
		}
	}
	if response := doRequest(env.base, "PUT", "/api/option/pricing", env.session, batch(0, "9", "9")); response.Status != 409 {
		t.Fatalf("stale version: %d %s", response.Status, response.Body)
	}
	if err := model.DB.Exec(`CREATE TRIGGER reject_price_batch BEFORE INSERT ON option_changes WHEN NEW.key = 'model_ratio' BEGIN SELECT RAISE(ABORT, 'synthetic history failure'); END`).Error; err != nil {
		t.Fatal(err)
	}
	if response := doRequest(env.base, "PUT", "/api/option/pricing", env.session, batch(2, "5", "6")); response.Status != 500 {
		t.Fatalf("second-key failure: %d %s", response.Status, response.Body)
	}
	if after := read(); after.Version != 2 || after.Loaded != 2 {
		t.Fatal("failed transaction advanced a version")
	}
	checkQuote(3, 12)
	if err := model.DB.Exec("DROP TRIGGER reject_price_batch").Error; err != nil {
		t.Fatal(err)
	}
	if err := model.UpsertOption("group_group_ratio", "corrupt-unrelated-option"); err != nil {
		t.Fatal(err)
	}
	response := doRequest(env.base, "PUT", "/api/option/pricing", env.session, batch(2, "5", "6"))
	if response.Status != 500 || !strings.Contains(response.Body, "saved, but serving settings could not be reloaded") {
		t.Fatalf("committed but unloaded outcome: %d %s", response.Status, response.Body)
	}
	if after := read(); after.Version != 4 || after.Loaded != 2 {
		t.Fatalf("committed and loaded versions were conflated: %+v", after)
	}
	checkQuote(3, 12)
	if response := doRequest(env.base, "PUT", "/api/option/pricing", env.session, batch(2, "5", "6")); response.Status != 409 {
		t.Fatal("unknown acknowledgement allowed a duplicate stale write")
	}
	if err := model.UpsertOption("group_group_ratio", "{}"); err != nil {
		t.Fatal(err)
	}
	if response := doRequest(env.base, "POST", "/api/option/reload", env.session, nil); response.Status != 200 {
		t.Fatalf("explicit reload: %d %s", response.Status, response.Body)
	}
	checkQuote(5, 30)
	changes, err := model.ListOptionChanges(100)
	if err != nil || len(changes) != 4 {
		t.Fatal("batch history count mismatch")
	}
	for _, change := range changes {
		if change.ActorID != env.rootID {
			t.Fatal("batch lost authenticated actor")
		}
	}
}
