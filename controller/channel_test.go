package controller

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"testing"

	"starhub/common"
	"starhub/model"
)

func mustChannel(t *testing.T, name, models, key string, status int) *model.Channel {
	t.Helper()
	ch := &model.Channel{
		Name:   name,
		Type:   common.DefaultChannelType,
		Key:    key,
		Models: models,
		Group:  common.DefaultGroup,
		Status: status,
		Weight: 1,
	}
	if err := model.InsertChannel(ch); err != nil {
		t.Fatalf("InsertChannel(%s) failed: %v", name, err)
	}
	return ch
}

func admin() (int64, int) { return 1, common.RoleAdmin }

func TestCreateChannelDefaults(t *testing.T) {
	newTestDB(t)
	uid, role := admin()

	rr := call(CreateChannel, testRequest{
		method: http.MethodPost,
		target: "/api/channel",
		body:   `{"name":"main","key":"sk-up","models":"gpt-4o"}`,
		userID: uid,
		role:   role,
	})

	var got map[string]any
	decodeData(t, rr, &got)
	if int(got["type"].(float64)) != common.DefaultChannelType {
		t.Fatalf("type = %v, want the default %d", got["type"], common.DefaultChannelType)
	}
	if int(got["status"].(float64)) != common.ChannelStatusEnabled {
		t.Fatalf("status = %v, want enabled", got["status"])
	}
	if got["weight"].(float64) != 1 {
		t.Fatalf("weight = %v, want 1", got["weight"])
	}
	if got["group"] != common.DefaultGroup {
		t.Fatalf("group = %v, want %q", got["group"], common.DefaultGroup)
	}
	if got["key_count"].(float64) != 1 {
		t.Fatalf("key_count = %v, want 1", got["key_count"])
	}

	// The channel must be discoverable through the ability table.
	if _, err := model.GetSatisfiedChannel(common.DefaultGroup, "gpt-4o"); err != nil {
		t.Fatalf("created channel is not selectable: %v", err)
	}
}

func TestCreateChannelRejectsUnsupportedType(t *testing.T) {
	newTestDB(t)
	uid, role := admin()

	rr := call(CreateChannel, testRequest{
		method: http.MethodPost,
		target: "/api/channel",
		body:   `{"name":"main","key":"sk-up","models":"gpt-4o","type":9999}`,
		userID: uid,
		role:   role,
	})
	// The message enumerates the supported types so the caller can fix it.
	expectError(t, rr, http.StatusBadRequest, "unsupported channel type 9999")
}

func TestCreateChannelRejectsEmptyModels(t *testing.T) {
	newTestDB(t)
	uid, role := admin()

	for _, models := range []string{`""`, `" , "`} {
		rr := call(CreateChannel, testRequest{
			method: http.MethodPost,
			target: "/api/channel",
			body:   fmt.Sprintf(`{"name":"main","key":"sk-up","models":%s}`, models),
			userID: uid,
			role:   role,
		})
		expectError(t, rr, http.StatusBadRequest, "models must be a non-empty")
	}
}

func TestCreateChannelNormalizesBaseURL(t *testing.T) {
	newTestDB(t)
	uid, role := admin()

	rr := call(CreateChannel, testRequest{
		method: http.MethodPost,
		target: "/api/channel",
		body:   `{"name":"main","key":"sk-up","models":"gpt-4o","base_url":"https://api.example.com/v1///"}`,
		userID: uid,
		role:   role,
	})
	var got map[string]any
	decodeData(t, rr, &got)
	if got["base_url"] != "https://api.example.com/v1" {
		t.Fatalf("base_url = %v, want the trailing slashes trimmed", got["base_url"])
	}

	rr = call(CreateChannel, testRequest{
		method: http.MethodPost,
		target: "/api/channel",
		body:   `{"name":"main","key":"sk-up","models":"gpt-4o","base_url":"ftp://api.example.com"}`,
		userID: uid,
		role:   role,
	})
	expectError(t, rr, http.StatusBadRequest, "base_url must start with http")
}

func TestCreateChannelRejectsBadKeyMode(t *testing.T) {
	newTestDB(t)
	uid, role := admin()

	rr := call(CreateChannel, testRequest{
		method: http.MethodPost,
		target: "/api/channel",
		body:   `{"name":"main","key":"sk-up","models":"gpt-4o","key_mode":"sticky"}`,
		userID: uid,
		role:   role,
	})
	expectError(t, rr, http.StatusBadRequest, "key_mode must be one of")

	// The documented modes are accepted and persisted.
	for _, mode := range []string{model.KeyModeRandom, model.KeyModeRoundRobin, ""} {
		rr = call(CreateChannel, testRequest{
			method: http.MethodPost,
			target: "/api/channel",
			body: fmt.Sprintf(`{"name":"m-%s","key":"sk-up","models":"gpt-4o","key_mode":%q}`,
				mode, mode),
			userID: uid,
			role:   role,
		})
		var got map[string]any
		decodeData(t, rr, &got)
		if got["key_mode"] != mode {
			t.Fatalf("key_mode = %v, want %q", got["key_mode"], mode)
		}
	}
}

func TestCreateChannelRejectsInvalidStatus(t *testing.T) {
	newTestDB(t)
	uid, role := admin()

	// Auto-disabled (3) is not settable through the API: only enabled and
	// manually-disabled are.
	rr := call(CreateChannel, testRequest{
		method: http.MethodPost,
		target: "/api/channel",
		body: fmt.Sprintf(`{"name":"main","key":"sk-up","models":"gpt-4o","status":%d}`,
			common.ChannelStatusAutoDisabled),
		userID: uid,
		role:   role,
	})
	expectError(t, rr, http.StatusBadRequest, "invalid status")
}

func TestCreateChannelNormalizesGroupList(t *testing.T) {
	newTestDB(t)
	uid, role := admin()

	rr := call(CreateChannel, testRequest{
		method: http.MethodPost,
		target: "/api/channel",
		body:   `{"name":"main","key":"sk-up","models":"a, b ,,c","group":" x , y "}`,
		userID: uid,
		role:   role,
	})
	var got map[string]any
	decodeData(t, rr, &got)
	if got["models"] != "a,b,c" {
		t.Fatalf("models = %v, want normalized a,b,c", got["models"])
	}
	if got["group"] != "x,y" {
		t.Fatalf("group = %v, want normalized x,y", got["group"])
	}
}

func TestGetChannelInvalidPathID(t *testing.T) {
	newTestDB(t)
	uid, role := admin()

	for _, raw := range []string{"", "abc", "0", "-1"} {
		rr := call(GetChannel, testRequest{
			target: "/api/channel/" + raw,
			userID: uid,
			role:   role,
			path:   map[string]string{"id": raw},
		})
		expectError(t, rr, http.StatusBadRequest, "invalid id in path")
	}
}

func TestGetChannelNotFound(t *testing.T) {
	newTestDB(t)
	uid, role := admin()

	rr := call(GetChannel, testRequest{
		target: "/api/channel/9999",
		userID: uid,
		role:   role,
		path:   map[string]string{"id": "9999"},
	})
	expectError(t, rr, http.StatusNotFound, "channel not found")
}

// A partial update must leave every field the caller omitted alone. The
// notable exception is `key`: replacing the keys clears the per-key disable
// state, because those statuses refer to keys that no longer exist.
func TestUpdateChannelPartialSemantics(t *testing.T) {
	newTestDB(t)
	uid, role := admin()
	ch := mustChannel(t, "main", "gpt-4o", "k1\nk2", common.ChannelStatusEnabled)
	ch.MultiKeyStatus = "2,1"
	if err := model.UpdateChannel(ch); err != nil {
		t.Fatalf("UpdateChannel failed: %v", err)
	}

	// Renaming only.
	rr := call(UpdateChannel, testRequest{
		method: http.MethodPut,
		target: "/api/channel",
		body:   fmt.Sprintf(`{"id":%d,"name":"renamed"}`, ch.ID),
		userID: uid,
		role:   role,
	})
	expectStatus(t, rr, http.StatusOK)
	got, err := model.GetChannelByID(ch.ID)
	if err != nil {
		t.Fatalf("GetChannelByID failed: %v", err)
	}
	if got.Name != "renamed" {
		t.Fatalf("name = %q, want renamed", got.Name)
	}
	if got.Models != "gpt-4o" {
		t.Fatalf("models = %q, want untouched gpt-4o", got.Models)
	}
	if got.Key != "k1\nk2" {
		t.Fatalf("key = %q, want untouched", got.Key)
	}
	if got.MultiKeyStatus != "2,1" {
		t.Fatalf("multi_key_status = %q, want untouched when the key is unchanged", got.MultiKeyStatus)
	}

	// Replacing the keys resets the stale per-key statuses.
	rr = call(UpdateChannel, testRequest{
		method: http.MethodPut,
		target: "/api/channel",
		body:   fmt.Sprintf(`{"id":%d,"key":"fresh-1\nfresh-2"}`, ch.ID),
		userID: uid,
		role:   role,
	})
	expectStatus(t, rr, http.StatusOK)
	got, _ = model.GetChannelByID(ch.ID)
	if got.Key != "fresh-1\nfresh-2" {
		t.Fatalf("key = %q, want the replacement", got.Key)
	}
	if got.MultiKeyStatus != "" {
		t.Fatalf("multi_key_status = %q, want cleared after a key swap", got.MultiKeyStatus)
	}
	var disabled int
	for _, s := range got.KeyStatuses() {
		if s != common.StatusEnabled {
			disabled++
		}
	}
	if disabled != 0 {
		t.Fatalf("%d fresh keys still carry a disabled status", disabled)
	}
}

func TestUpdateChannelValidationErrors(t *testing.T) {
	newTestDB(t)
	uid, role := admin()
	ch := mustChannel(t, "main", "gpt-4o", "sk-up", common.ChannelStatusEnabled)
	id := itoa(ch.ID)

	cases := []struct {
		name string
		body string
		want string
	}{
		{"missing id", `{"name":"x"}`, "channel id is required"},
		{"unknown id", `{"id":9999,"name":"x"}`, "channel not found"},
		{"bad key_mode", fmt.Sprintf(`{"id":%s,"key_mode":"nope"}`, id), "key_mode must be one of"},
		{"bad base_url", fmt.Sprintf(`{"id":%s,"base_url":"ftp://x"}`, id), "base_url must start with http"},
		{"empty models", fmt.Sprintf(`{"id":%s,"models":" , "}`, id), "models must be a non-empty"},
		{"empty group", fmt.Sprintf(`{"id":%s,"group":" , "}`, id), "invalid group"},
		{"bad status", fmt.Sprintf(`{"id":%s,"status":%d}`, id, common.ChannelStatusAutoDisabled), "invalid status"},
	}
	for _, tc := range cases {
		rr := call(UpdateChannel, testRequest{
			method: http.MethodPut,
			target: "/api/channel",
			body:   tc.body,
			userID: uid,
			role:   role,
		})
		status := http.StatusBadRequest
		if tc.name == "unknown id" {
			status = http.StatusNotFound
		}
		expectError(t, rr, status, tc.want)
	}

	// None of the rejected updates may have touched the row.
	got, err := model.GetChannelByID(ch.ID)
	if err != nil {
		t.Fatalf("GetChannelByID failed: %v", err)
	}
	if got.KeyMode != "" || got.BaseURL != "" || got.Models != "gpt-4o" || got.Status != common.ChannelStatusEnabled {
		t.Fatalf("a rejected update mutated the channel: %+v", got)
	}
}

func TestDeleteChannelNotFoundHandler(t *testing.T) {
	newTestDB(t)
	uid, role := admin()

	rr := call(DeleteChannel, testRequest{
		method: http.MethodDelete,
		target: "/api/channel/9999",
		userID: uid,
		role:   role,
		path:   map[string]string{"id": "9999"},
	})
	expectError(t, rr, http.StatusNotFound, "channel not found")
}

func TestDeleteChannelRemovesFromSelection(t *testing.T) {
	newTestDB(t)
	uid, role := admin()
	ch := mustChannel(t, "main", "gpt-4o", "sk-up", common.ChannelStatusEnabled)

	rr := call(DeleteChannel, testRequest{
		method: http.MethodDelete,
		target: "/api/channel/" + itoa(ch.ID),
		userID: uid,
		role:   role,
		path:   map[string]string{"id": itoa(ch.ID)},
	})
	expectStatus(t, rr, http.StatusOK)
	if _, err := model.GetSatisfiedChannel(common.DefaultGroup, "gpt-4o"); err != model.ErrNoChannel {
		t.Fatalf("deleted channel is still selectable: %v", err)
	}
}

// TestChannel only validates the request; the probe itself needs a live
// upstream, so these tests assert the guards that run before any network I/O.
func TestTestChannelGuards(t *testing.T) {
	newTestDB(t)
	uid, role := admin()
	ch := mustChannel(t, "main", "gpt-4o,claude-3", "sk-up", common.ChannelStatusEnabled)
	id := itoa(ch.ID)

	rr := call(TestChannel, testRequest{
		method: http.MethodPost,
		target: "/api/channel/test/9999",
		userID: uid,
		role:   role,
		path:   map[string]string{"id": "9999"},
	})
	expectError(t, rr, http.StatusNotFound, "channel not found")

	rr = call(TestChannel, testRequest{
		method: http.MethodPost,
		target: "/api/channel/test/" + id + "?model=not-in-list",
		userID: uid,
		role:   role,
		path:   map[string]string{"id": id},
	})
	expectError(t, rr, http.StatusBadRequest, "does not belong to this channel")

	// A channel with no models cannot be probed at all.
	empty := mustChannel(t, "empty", "gpt-4o", "sk-up", common.ChannelStatusEnabled)
	if err := model.DB.Model(&model.Channel{}).Where("id = ?", empty.ID).
		Update("models", "").Error; err != nil {
		t.Fatalf("clearing models failed: %v", err)
	}
	rr = call(TestChannel, testRequest{
		method: http.MethodPost,
		target: "/api/channel/test/" + itoa(empty.ID),
		userID: uid,
		role:   role,
		path:   map[string]string{"id": itoa(empty.ID)},
	})
	expectError(t, rr, http.StatusBadRequest, "has no models")
}

func TestMaskedChannelHidesKeyAndCountsKeys(t *testing.T) {
	newTestDB(t)
	uid, role := admin()
	ch := mustChannel(t, "multi", "gpt-4o", "secret-1\nsecret-2\nsecret-3", common.ChannelStatusEnabled)

	rr := call(GetChannel, testRequest{
		target: "/api/channel/" + itoa(ch.ID),
		userID: uid,
		role:   role,
		path:   map[string]string{"id": itoa(ch.ID)},
	})

	var got map[string]any
	decodeData(t, rr, &got)
	masked := got["key"].(string)
	for _, key := range ch.Keys() {
		if strings.Contains(masked, key) {
			t.Fatalf("the raw upstream key %q was returned in %q", key, masked)
		}
	}
	if got["key_count"].(float64) != 3 {
		t.Fatalf("key_count = %v, want 3", got["key_count"])
	}
	statuses := got["multi_key_status"].([]any)
	if len(statuses) != 3 {
		t.Fatalf("multi_key_status has %d entries, want 3 (aligned with the keys)", len(statuses))
	}
	for _, s := range statuses {
		if int(s.(float64)) != common.StatusEnabled {
			t.Fatalf("fresh multi-key channel reports a disabled key: %v", statuses)
		}
	}
}

func TestFormatChannelTypesIsSortedAndStable(t *testing.T) {
	out := formatChannelTypes()
	if out == "" {
		t.Fatal("formatChannelTypes returned an empty string")
	}
	// Stable ordering matters: the string is echoed back in API errors, so a
	// map iteration order would make the message flap.
	if again := formatChannelTypes(); again != out {
		t.Fatalf("formatChannelTypes is not deterministic:\n%s\n%s", out, again)
	}
	previous := -1
	for _, entry := range strings.Split(out, ", ") {
		idText, _, ok := strings.Cut(entry, " (")
		if !ok {
			t.Fatalf("channel type entry has no ID: %q", entry)
		}
		id, err := strconv.Atoi(idText)
		if err != nil {
			t.Fatalf("channel type entry has invalid ID %q: %v", idText, err)
		}
		if id <= previous {
			t.Fatalf("channel type IDs are not strictly ascending: %q", out)
		}
		previous = id
	}
}

func TestNormalizeKeyModeAndBaseURLHelpers(t *testing.T) {
	rr := &testRecorder{}
	if mode, ok := normalizeKeyMode(rr, "  "); !ok || mode != "" {
		t.Fatalf("blank key_mode = %q, %v; want empty and accepted", mode, ok)
	}
	if _, ok := normalizeKeyMode(rr, "sticky"); ok {
		t.Fatal("unknown key_mode should be rejected")
	}
	if rr.status != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rr.status)
	}

	rr = &testRecorder{}
	if base, ok := normalizeBaseURL(rr, "  https://x/y/  "); !ok || base != "https://x/y" {
		t.Fatalf("normalizeBaseURL = %q, %v", base, ok)
	}
	if base, ok := normalizeBaseURL(rr, ""); !ok || base != "" {
		t.Fatalf("empty base_url should be accepted as-is, got %q %v", base, ok)
	}
}

func TestModelInList(t *testing.T) {
	list := []string{"gpt-4o", "claude-3"}
	if !modelInList("gpt-4o", list) {
		t.Fatal("exact match should be found")
	}
	if modelInList("gpt-4", list) || modelInList("", list) {
		t.Fatal("partial or empty names must not match")
	}
}
