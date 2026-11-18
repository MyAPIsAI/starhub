package controller

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"starhub/common"
	"starhub/model"
)

func TestSetChannelStatusValidationAndMaskedResponse(t *testing.T) {
	newTestDB(t)
	ch := mustChannel(t, "multi", "alpha", "secret-first-key\nsecret-second-key", common.ChannelStatusEnabled)
	invoke := func(id, body string) *httptest.ResponseRecorder {
		return call(SetChannelStatus, testRequest{
			method: http.MethodPut, target: "/api/channel/" + id + "/status", body: body,
			userID: 1, role: common.RoleAdmin, path: map[string]string{"id": id},
		})
	}
	for _, body := range []string{`{}`, `{"enabled":null}`, `{"enabled":1}`, `{"enabled":"false"}`} {
		if rr := invoke(fmt.Sprint(ch.ID), body); rr.Code != http.StatusBadRequest {
			t.Fatalf("invalid availability request was accepted: %s", rr.Body)
		}
	}
	expectError(t, invoke("abc", `{"enabled":false}`), http.StatusBadRequest, "invalid id")
	expectError(t, invoke("99999", `{"enabled":true}`), http.StatusNotFound, "channel not found")
	for _, enabled := range []bool{false, true} {
		rr := invoke(fmt.Sprint(ch.ID), fmt.Sprintf(`{"enabled":%t}`, enabled))
		var response map[string]any
		decodeData(t, rr, &response)
		for _, key := range ch.Keys() {
			if strings.Contains(rr.Body.String(), key) {
				t.Fatal("availability response leaked an upstream credential")
			}
		}
		stored, _ := model.GetChannelByID(ch.ID)
		if !reflect.DeepEqual(response, maskedChannelJSON(t, stored)) {
			t.Fatal("availability response differs from the public channel shape")
		}
	}
}

func maskedChannelJSON(t *testing.T, ch *model.Channel) map[string]any {
	t.Helper()
	data, err := json.Marshal(maskedChannel(ch))
	if err != nil {
		t.Fatal(err)
	}
	var result map[string]any
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatal(err)
	}
	return result
}
