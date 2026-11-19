package controller

import (
	"net/http"
	"strings"
	"testing"

	"starhub/model"
)

func TestPublicStatusOverviewAnonymousAndReadFailure(t *testing.T) {
	newTestDB(t)
	mustChannel(t, "public status", "fixture-model", "fixture-secret", 1)
	var view model.PublicStatusOverview
	decodeData(t, call(PublicStatusOverviewHandler, testRequest{target: "/api/status/overview"}), &view)
	if len(view.Models) != 1 || view.Models[0].ID != "fixture-model" {
		t.Fatal(view)
	}
	if err := model.DB.Migrator().DropTable(&model.Ability{}); err != nil {
		t.Fatal(err)
	}
	response := call(PublicStatusOverviewHandler, testRequest{target: "/api/status/overview"})
	if response.Code != http.StatusServiceUnavailable || strings.Contains(response.Body.String(), "abilities") {
		t.Fatal(response.Body)
	}
}
