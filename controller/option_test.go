package controller

import (
	"fmt"
	"net/http"
	"testing"

	"starhub/common"
	"starhub/model"
	"starhub/setting"
)

func TestUpdateOptionRejectsUnknownKey(t *testing.T) {
	newTestDB(t)
	uid, role := admin()

	rr := call(UpdateOption, testRequest{
		method: http.MethodPut,
		target: "/api/option",
		body:   `{"key":"not_an_option","value":"1"}`,
		userID: uid,
		role:   role,
	})
	expectError(t, rr, http.StatusBadRequest, "unknown option key")
}

func TestUpdateOptionRejectsInvalidBody(t *testing.T) {
	newTestDB(t)
	uid, role := admin()

	rr := call(UpdateOption, testRequest{
		method: http.MethodPut,
		target: "/api/option",
		body:   `{"key":`,
		userID: uid,
		role:   role,
	})
	expectError(t, rr, http.StatusBadRequest, "invalid request body")
}

// A value the setting layer refuses must be a 400 and must not be written
// through to the option table.
func TestUpdateOptionRejectsInvalidValue(t *testing.T) {
	newTestDB(t)
	uid, role := admin()

	// Every option key is seeded on startup, so capture the pre-update value
	// and assert it is untouched rather than assuming an empty table.
	before, found, err := model.GetOption(setting.OptionKeyCacheRatio)
	if err != nil {
		t.Fatalf("GetOption failed: %v", err)
	}
	if !found {
		t.Fatal("cache_ratio should have been seeded")
	}

	rr := call(UpdateOption, testRequest{
		method: http.MethodPut,
		target: "/api/option",
		body:   fmt.Sprintf(`{"key":%q,"value":"not-a-number"}`, setting.OptionKeyCacheRatio),
		userID: uid,
		role:   role,
	})
	expectError(t, rr, http.StatusBadRequest, "invalid value")

	after, _, err := model.GetOption(setting.OptionKeyCacheRatio)
	if err != nil {
		t.Fatalf("GetOption failed: %v", err)
	}
	if after != before {
		t.Fatalf("option changed from %q to %q despite the validation error", before, after)
	}
	if got := setting.CacheRatioValue(); got != setting.FallbackCacheRatio {
		t.Fatalf("in-memory cache ratio = %v, want the untouched default %v", got, setting.FallbackCacheRatio)
	}
}

func TestUpdateOptionDatabaseFailureLeavesMemoryUnchanged(t *testing.T) {
	newTestDB(t)
	uid, role := admin()
	before, _, err := model.GetOption(setting.OptionKeyCacheRatio)
	if err != nil {
		t.Fatal(err)
	}
	if err := model.DB.Create(&model.OptionChange{Version: 1, Key: "reserved"}).Error; err != nil {
		t.Fatal(err)
	}
	rr := call(UpdateOption, testRequest{method: http.MethodPut, target: "/api/option",
		body: fmt.Sprintf(`{"key":%q,"value":"2.5"}`, setting.OptionKeyCacheRatio), userID: uid, role: role})
	expectStatus(t, rr, http.StatusInternalServerError)
	after, _, err := model.GetOption(setting.OptionKeyCacheRatio)
	if err != nil {
		t.Fatal(err)
	}
	if after != before || setting.CacheRatioValue() != setting.FallbackCacheRatio {
		t.Fatalf("failed transaction changed option: db=%q memory=%v", after, setting.CacheRatioValue())
	}
}

func TestOptionVersionHistoryAndRollback(t *testing.T) {
	newTestDB(t)
	uid, role := admin()
	rr := call(UpdateOption, testRequest{method: http.MethodPut, target: "/api/option",
		body:   fmt.Sprintf(`{"key":%q,"value":"2.5","expected_version":0}`, setting.OptionKeyCacheRatio),
		userID: uid, role: role})
	expectStatus(t, rr, http.StatusOK)
	rr = call(OptionStatus, testRequest{target: "/api/option/status", userID: uid, role: role})
	var status struct {
		DatabaseVersion int64 `json:"database_version"`
		LoadedVersion   int64 `json:"loaded_version"`
		InSync          bool  `json:"in_sync"`
	}
	decodeData(t, rr, &status)
	if status.DatabaseVersion != 1 || status.LoadedVersion != 1 || !status.InSync {
		t.Fatalf("option status = %+v", status)
	}
	rr = call(UpdateOption, testRequest{method: http.MethodPut, target: "/api/option",
		body:   fmt.Sprintf(`{"key":%q,"value":"3","expected_version":0}`, setting.OptionKeyCacheRatio),
		userID: uid, role: role})
	expectError(t, rr, http.StatusConflict, "option version changed")
	rr = call(OptionHistory, testRequest{target: "/api/option/history", userID: uid, role: role})
	var history []model.OptionChange
	decodeData(t, rr, &history)
	if len(history) != 1 || history[0].OldValue != "0.5" || history[0].NewValue != "2.5" {
		t.Fatalf("option history = %+v", history)
	}
	rr = call(RollbackOption, testRequest{method: http.MethodPost, target: "/api/option/rollback",
		body: `{"version":1,"expected_version":1}`, userID: uid, role: role})
	expectStatus(t, rr, http.StatusOK)
	if got := setting.CacheRatioValue(); got != setting.FallbackCacheRatio {
		t.Fatalf("cache ratio after rollback = %v", got)
	}
}

func TestWebhookOptionIsRedacted(t *testing.T) {
	newTestDB(t)
	uid, role := admin()
	rr := call(UpdateOption, testRequest{method: http.MethodPut, target: "/api/option",
		body: `{"key":"alert_webhook_url","value":"https://example.test/secret"}`, userID: uid, role: role})
	expectStatus(t, rr, http.StatusOK)
	rr = call(ListOptions, testRequest{target: "/api/option", userID: uid, role: role})
	var options []model.Option
	decodeData(t, rr, &options)
	for _, option := range options {
		if option.Key == setting.OptionKeyAlertWebhookURL && option.Value != "[redacted]" {
			t.Fatalf("webhook URL was exposed: %q", option.Value)
		}
	}
	rr = call(OptionHistory, testRequest{target: "/api/option/history", userID: uid, role: role})
	var history []model.OptionChange
	decodeData(t, rr, &history)
	if len(history) != 1 || history[0].OldValue != "[redacted]" || history[0].NewValue != "[redacted]" {
		t.Fatalf("webhook history was exposed: %+v", history)
	}
}

func TestUpdateOptionPersistsAndReloads(t *testing.T) {
	newTestDB(t)
	uid, role := admin()

	rr := call(UpdateOption, testRequest{
		method: http.MethodPut,
		target: "/api/option",
		body:   fmt.Sprintf(`{"key":%q,"value":"2.5"}`, setting.OptionKeyCacheRatio),
		userID: uid,
		role:   role,
	})
	expectStatus(t, rr, http.StatusOK)

	// The in-memory setting moved...
	if got := setting.CacheRatioValue(); got != 2.5 {
		t.Fatalf("setting cache ratio = %v, want 2.5", got)
	}
	// ...and the value survived a round trip through the database.
	stored, found, err := model.GetOption(setting.OptionKeyCacheRatio)
	if err != nil {
		t.Fatalf("GetOption failed: %v", err)
	}
	if !found || stored != "2.5" {
		t.Fatalf("stored option = %q (found %v), want 2.5", stored, found)
	}

	// A reload from the database reproduces the same effective setting.
	setting.ResetAllToDefaults()
	if err := model.LoadOptionsToSettings(); err != nil {
		t.Fatalf("LoadOptionsToSettings failed: %v", err)
	}
	if got := setting.CacheRatioValue(); got != 2.5 {
		t.Fatalf("after reload cache ratio = %v, want 2.5", got)
	}
}

// Ratio options are stored in their canonical snapshot form rather than the
// raw request value, so the persisted map always covers every built-in model.
func TestUpdateOptionRatioRoundTripsThroughSnapshot(t *testing.T) {
	newTestDB(t)
	uid, role := admin()

	rr := call(UpdateOption, testRequest{
		method: http.MethodPut,
		target: "/api/option",
		body:   fmt.Sprintf(`{"key":%q,"value":"{\"gpt-4o\":9.5}"}`, setting.OptionKeyModelRatio),
		userID: uid,
		role:   role,
	})
	expectStatus(t, rr, http.StatusOK)

	want, err := setting.SnapshotRatio(setting.OptionKeyModelRatio)
	if err != nil {
		t.Fatalf("SnapshotRatio failed: %v", err)
	}
	stored, found, err := model.GetOption(setting.OptionKeyModelRatio)
	if err != nil {
		t.Fatalf("GetOption failed: %v", err)
	}
	if !found {
		t.Fatal("model_ratio was not persisted")
	}
	if stored != want {
		t.Fatalf("stored model_ratio differs from the snapshot:\n got %s\nwant %s", stored, want)
	}
	if got := setting.ModelRatioValue("gpt-4o"); got != 9.5 {
		t.Fatalf("gpt-4o ratio = %v, want the updated 9.5", got)
	}
}

// The ratio snapshot must retain pricing for models the caller did not
// mention, otherwise a partial update would silently reset them.
func TestUpdateOptionRatioKeepsUnmentionedModels(t *testing.T) {
	newTestDB(t)
	uid, role := admin()

	before := setting.ModelRatioValue("gpt-4o-mini")

	rr := call(UpdateOption, testRequest{
		method: http.MethodPut,
		target: "/api/option",
		body:   fmt.Sprintf(`{"key":%q,"value":"{\"gpt-4o\":1.5}"}`, setting.OptionKeyModelRatio),
		userID: uid,
		role:   role,
	})
	expectStatus(t, rr, http.StatusOK)

	if got := setting.ModelRatioValue("gpt-4o-mini"); got != before {
		t.Fatalf("gpt-4o-mini ratio = %v, want the untouched %v", got, before)
	}
}

func TestListOptionsReturnsSeededKeys(t *testing.T) {
	newTestDB(t)
	uid, role := admin()

	rr := call(ListOptions, testRequest{target: "/api/option", userID: uid, role: role})
	var got []map[string]string
	decodeData(t, rr, &got)

	if len(got) == 0 {
		t.Fatal("no options were seeded")
	}
	seen := map[string]bool{}
	for _, o := range got {
		seen[o["key"]] = true
	}
	// SeedOptions writes every key the setting package owns.
	for _, key := range setting.OptionKeys() {
		if !seen[key] {
			t.Fatalf("seeded option %q is missing from the listing", key)
		}
	}
}

func TestUpdateOptionAffiliateRoundTrip(t *testing.T) {
	newTestDB(t)
	uid, role := admin()

	rr := call(UpdateOption, testRequest{
		method: http.MethodPut,
		target: "/api/option",
		body:   fmt.Sprintf(`{"key":%q,"value":"250"}`, setting.OptionKeyAffRegisterBonus),
		userID: uid,
		role:   role,
	})
	expectStatus(t, rr, http.StatusOK)

	if got := setting.AffRegisterBonusValue(); got != 250 {
		t.Fatalf("register bonus = %d, want 250", got)
	}

	// A negative bonus is meaningless and must be refused.
	rr = call(UpdateOption, testRequest{
		method: http.MethodPut,
		target: "/api/option",
		body:   fmt.Sprintf(`{"key":%q,"value":"-1"}`, setting.OptionKeyAffRegisterBonus),
		userID: uid,
		role:   role,
	})
	expectError(t, rr, http.StatusBadRequest, "invalid value")
	if got := setting.AffRegisterBonusValue(); got != 250 {
		t.Fatalf("register bonus = %d, want the rejected update to leave 250", got)
	}
}

// The ratio option is hot-swappable: a rebate ratio above 1 would let an
// inviter earn more than the invitee spends.
func TestUpdateOptionRebateRatioBounds(t *testing.T) {
	newTestDB(t)
	uid, role := admin()

	for _, value := range []string{"-0.1", "1.5"} {
		rr := call(UpdateOption, testRequest{
			method: http.MethodPut,
			target: "/api/option",
			body:   fmt.Sprintf(`{"key":%q,"value":%q}`, setting.OptionKeyAffRebateRatio, value),
			userID: uid,
			role:   role,
		})
		expectError(t, rr, http.StatusBadRequest, "invalid value")
	}

	rr := call(UpdateOption, testRequest{
		method: http.MethodPut,
		target: "/api/option",
		body:   fmt.Sprintf(`{"key":%q,"value":"0.25"}`, setting.OptionKeyAffRebateRatio),
		userID: uid,
		role:   role,
	})
	expectStatus(t, rr, http.StatusOK)
	if got := setting.AffRebateRatioValue(); got != 0.25 {
		t.Fatalf("rebate ratio = %v, want 0.25", got)
	}
	_ = common.RoleAdmin
}
