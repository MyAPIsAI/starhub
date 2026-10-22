package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"reflect"
	"strings"
	"testing"

	"starhub/common"
	"starhub/middleware"
	"starhub/model"
)

func exportTestUser(t *testing.T, baseURL, name string) (int64, string) {
	t.Helper()
	password, err := common.HashPassword("export-password")
	if err != nil {
		t.Fatal(err)
	}
	u := &model.User{Username: name, Password: password, Role: common.RoleUser, Status: common.StatusEnabled, Group: common.DefaultGroup, AffCode: "invite-" + name}
	if err := model.CreateUser(u); err != nil {
		t.Fatal(err)
	}
	res := doRequest(baseURL, http.MethodPost, "/api/user/login", "", map[string]string{"username": name, "password": "export-password"})
	var got struct {
		Success bool `json:"success"`
		Data    struct {
			Session string     `json:"session"`
			User    model.User `json:"user"`
		} `json:"data"`
	}
	if err := json.Unmarshal([]byte(res.Body), &got); err != nil || res.Status != http.StatusOK || !got.Success || got.Data.Session == "" || got.Data.User.ID != u.ID || got.Data.User.Role != common.RoleUser {
		t.Fatalf("login %s failed: %d %s (decode error %v)", name, res.Status, res.Body, err)
	}
	t.Cleanup(func() { middleware.Sessions.Remove(got.Data.Session) })
	return u.ID, got.Data.Session
}

func TestExportDownloadUserIsolation(t *testing.T) {
	env := setupRelayApp(t)
	t.Cleanup(func() { middleware.Sessions.Remove(env.session) })
	aliceID, aliceSession := exportTestUser(t, env.base, "export-alice")
	_, bobSession := exportTestUser(t, env.base, "export-bob")
	const privateName = "alice-private-export-key"
	if err := model.InsertLog(&model.Log{UserID: aliceID, TokenName: privateName, Model: "gpt-4o-mini", PromptTokens: 100, CompletionTokens: 50, Quota: 23}); err != nil {
		t.Fatal(err)
	}
	res := doRequest(env.base, http.MethodPost, "/api/export", aliceSession, map[string]any{"type": "requests", "range_days": 7})
	var created struct {
		Success bool               `json:"success"`
		Data    model.ExportRecord `json:"data"`
	}
	if err := json.Unmarshal([]byte(res.Body), &created); err != nil || res.Status != http.StatusOK || !created.Success || created.Data.ID <= 0 || created.Data.UserID != aliceID || created.Data.RowCount != 1 {
		t.Fatalf("create export failed: %d %s (decode error %v)", res.Status, res.Body, err)
	}
	stored, err := model.GetExport(created.Data.ID)
	if err != nil {
		t.Fatal(err)
	}
	path := fmt.Sprintf("/api/export/%d/download", created.Data.ID)
	cases := []struct {
		name    string
		session string
		status  int
		message string
	}{
		{"anonymous", "", http.StatusUnauthorized, "login required"},
		{"other user", bobSession, http.StatusForbidden, "cannot download an export owned by another user"},
		{"owner", aliceSession, http.StatusOK, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			res := doRequest(env.base, http.MethodGet, path, tc.session, nil)
			if res.Status != tc.status {
				t.Fatalf("download status = %d, want %d (body %q)", res.Status, tc.status, res.Body)
			}
			if tc.status == http.StatusOK {
				if res.Body != string(stored.Content) || !strings.Contains(res.Body, privateName) {
					t.Fatalf("owner did not receive the persisted private CSV: %q", res.Body)
				}
			} else {
				var denied common.APIResponse
				if err := json.Unmarshal([]byte(res.Body), &denied); err != nil || denied.Success || denied.Message != tc.message || strings.Contains(res.Body, privateName) {
					t.Fatalf("unexpected denial response: %s (decode error %v)", res.Body, err)
				}
			}
			after, err := model.ListExports(0)
			if err != nil {
				t.Fatal(err)
			}
			if want := []model.ExportRecord{*stored}; !reflect.DeepEqual(after, want) {
				t.Fatalf("download changed export records: got %+v, want %+v", after, want)
			}
		})
	}
}

func TestExportListUserIsolation(t *testing.T) {
	env := setupRelayApp(t)
	t.Cleanup(func() { middleware.Sessions.Remove(env.session) })
	aliceID, aliceSession := exportTestUser(t, env.base, "export-alice")
	bobID, bobSession := exportTestUser(t, env.base, "export-bob")
	aliceExport := &model.ExportRecord{UserID: aliceID, Type: "requests", Filename: "alice-private.csv", RowCount: 1, Content: []byte("alice-private-content")}
	bobExport := &model.ExportRecord{UserID: bobID, Type: "usage", Filename: "bob-private.csv", RowCount: 2, Content: []byte("bob-private-content")}
	for _, rec := range []*model.ExportRecord{aliceExport, bobExport} {
		if err := model.InsertExport(rec); err != nil {
			t.Fatal(err)
		}
	}
	before, err := model.ListExports(0)
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name    string
		session string
		want    *model.ExportRecord
	}{
		{"anonymous", "", nil},
		{"alice", aliceSession, aliceExport},
		{"bob", bobSession, bobExport},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			res := doRequest(env.base, http.MethodGet, "/api/export", tc.session, nil)
			var got struct {
				Success bool                 `json:"success"`
				Message string               `json:"message"`
				Data    []model.ExportRecord `json:"data"`
			}
			if err := json.Unmarshal([]byte(res.Body), &got); err != nil {
				t.Fatalf("invalid list response: %s (decode error %v)", res.Body, err)
			}
			if tc.want == nil {
				if res.Status != http.StatusUnauthorized || got.Success || got.Message != "login required" || len(got.Data) != 0 {
					t.Fatalf("unexpected anonymous response: %d %s", res.Status, res.Body)
				}
			} else {
				if res.Status != http.StatusOK || !got.Success || len(got.Data) != 1 {
					t.Fatalf("expected exactly one owned export: %d %s", res.Status, res.Body)
				}
				want := *tc.want
				want.Content = nil
				want.ExpiresAt = 0
				if !reflect.DeepEqual(got.Data[0], want) {
					t.Fatalf("listed export = %+v, want owned metadata %+v", got.Data[0], want)
				}
			}
			if strings.Contains(res.Body, "alice-private-content") || strings.Contains(res.Body, "bob-private-content") {
				t.Fatalf("list response leaked CSV content: %s", res.Body)
			}
			after, err := model.ListExports(0)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(after, before) {
				t.Fatalf("listing changed unexpired export records: got %+v, want %+v", after, before)
			}
		})
	}
}
