package main

import (
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"starhub/common"
	"starhub/middleware"
	"starhub/model"
)

func TestModelMetadataCatalogRoutesRejectStaleEditsAndSameNameRecreation(t *testing.T) {
	env := setupRelayApp(t)
	read := func() struct {
		Revision int64             `json:"revision"`
		Guards   bool              `json:"revision_guards"`
		Models   []model.ModelMeta `json:"models"`
	} {
		t.Helper()
		response := doRequest(env.base, "GET", "/api/plaza/meta/configuration", env.session, nil)
		var body struct {
			Data struct {
				Revision int64             `json:"revision"`
				Guards   bool              `json:"revision_guards"`
				Models   []model.ModelMeta `json:"models"`
			} `json:"data"`
		}
		if err := json.Unmarshal([]byte(response.Body), &body); err != nil || response.Status != 200 {
			t.Fatalf("catalog: %d %s %v", response.Status, response.Body, err)
		}
		return body.Data
	}
	initial := read()
	if initial.Revision != 0 || !initial.Guards || initial.Models == nil || len(initial.Models) != 0 {
		t.Fatal("empty catalog lacks explicit revision support")
	}
	write := func(revision int64, description string) apiResult {
		return doRequest(env.base, "PUT", "/api/plaza/meta", env.session, map[string]any{"model": "guarded/custom", "description": description, "expected_catalog_revision": revision, "tags": "function_call,unknown-extension", "max_output": 2048})
	}
	if response := write(0, "Original"); response.Status != 200 {
		t.Fatalf("create: %d %s", response.Status, response.Body)
	}
	state := read()
	legacy := state.Models[0]
	legacy.Description = "Legacy update"
	if response := doRequest(env.base, "PUT", "/api/plaza/meta", env.session, legacy); response.Status != 200 {
		t.Fatalf("legacy write: %d %s", response.Status, response.Body)
	}
	if response := write(1, "Must not overwrite legacy"); response.Status != 409 {
		t.Fatalf("stale save: %d %s", response.Status, response.Body)
	}
	deleteAt := func(revision int64) apiResult {
		return doRequest(env.base, "DELETE", "/api/plaza/meta/guarded/custom", env.session, map[string]any{"expected_catalog_revision": revision})
	}
	if response := deleteAt(1); response.Status != 409 {
		t.Fatalf("stale delete: %d %s", response.Status, response.Body)
	}
	if response := deleteAt(2); response.Status != 200 {
		t.Fatalf("delete: %d %s", response.Status, response.Body)
	}
	if response := write(3, "Recreated"); response.Status != 200 {
		t.Fatalf("recreate: %d %s", response.Status, response.Body)
	}
	if response := deleteAt(2); response.Status != 409 {
		t.Fatal("stale delete removed a recreated model")
	}
	for _, expected := range []any{nil, -1, 1.5, "4", int64(1 << 53), int64(1<<53 - 1)} {
		if response := doRequest(env.base, "PUT", "/api/plaza/meta", env.session, map[string]any{"model": "invalid", "expected_catalog_revision": expected}); response.Status != 400 {
			t.Fatalf("invalid save guard: %d %s", response.Status, response.Body)
		}
		if response := doRequest(env.base, "DELETE", "/api/plaza/meta/guarded/custom", env.session, map[string]any{"expected_catalog_revision": expected}); response.Status != 400 {
			t.Fatalf("invalid delete guard: %d %s", response.Status, response.Body)
		}
	}
	if response := doRequest(env.base, "DELETE", "/api/plaza/meta/guarded/custom", env.session, map[string]any{}); response.Status != 400 {
		t.Fatal("delete body without a precondition was ignored")
	}
	if err := model.DB.Exec(`CREATE TRIGGER reject_metadata_catalog BEFORE UPDATE ON model_metadata_states WHEN NEW.revision > OLD.revision BEGIN SELECT RAISE(ABORT, 'synthetic catalog failure'); END`).Error; err != nil {
		t.Fatal(err)
	}
	if response := write(4, "Must roll back"); response.Status != 500 {
		t.Fatal("revision storage failure was ignored")
	}
	if response := deleteAt(4); response.Status != 500 {
		t.Fatal("failed delete revision was ignored")
	}
	state = read()
	if state.Revision != 4 || len(state.Models) != 1 || state.Models[0].Description != "Recreated" || state.Models[0].MaxOutput != 2048 {
		t.Fatalf("guarded API changed the wrong metadata: %+v", state)
	}
	if version, err := model.OptionVersion(); err != nil || version != 0 {
		t.Fatal("metadata operations changed billing-option versions")
	}
}

func TestModelMetadataCatalogUsesActualAdministrativeRoles(t *testing.T) {
	env := setupRelayApp(t)
	for _, role := range []int{common.RoleUser, common.RoleFinance, 99, 101, common.RoleAdmin, common.RoleRoot} {
		user := &model.User{Username: fmt.Sprintf("metadata-role-%d", role), Role: role, Status: common.StatusEnabled, AffCode: common.GenerateAffCode()}
		if err := model.CreateUser(user); err != nil {
			t.Fatal(err)
		}
		session := middleware.Sessions.Create(user.ID, time.Hour)
		t.Cleanup(func() { middleware.Sessions.Remove(session) })
		want := 403
		if role == common.RoleAdmin || role == common.RoleRoot {
			want = 200
		}
		response := doRequest(env.base, "GET", "/api/plaza/meta/configuration", session, nil)
		if response.Status != want {
			t.Fatalf("role %d catalog=%d", role, response.Status)
		}
		state, err := model.ReadModelMetadataCatalog(t.Context())
		if err != nil {
			t.Fatal(err)
		}
		response = doRequest(env.base, "PUT", "/api/plaza/meta", session, map[string]any{"model": fmt.Sprintf("role-%d", role), "expected_catalog_revision": state.Revision})
		if response.Status != want {
			t.Fatalf("role %d guarded write=%d %s", role, response.Status, response.Body)
		}
	}
	for _, method := range []string{"GET", "PUT"} {
		path := "/api/plaza/meta"
		if method == "GET" {
			path += "/configuration"
		}
		if response := doRequest(env.base, method, path, "", map[string]any{"model": "anonymous", "expected_catalog_revision": 0}); response.Status != 401 {
			t.Fatalf("anonymous %s=%d", method, response.Status)
		}
	}
}
