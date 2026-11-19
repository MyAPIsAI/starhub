package model

import (
	"encoding/json"
	"strings"
	"testing"

	"starhub/common"
	"starhub/setting"
)

func TestPublicStatusCatalogScopeAndClosedResources(t *testing.T) {
	newTestDB(t)
	setting.ResetAllToDefaults()
	t.Cleanup(setting.ResetAllToDefaults)
	if err := SeedOptions(); err != nil {
		t.Fatal(err)
	}
	if _, err := SaveOption(setting.OptionKeyGroupRatio, `{"default":1,"private":1}`, 0); err != nil {
		t.Fatal(err)
	}
	if err := LoadOptionsToSettings(); err != nil {
		t.Fatal(err)
	}
	if err := UpsertServiceGroup(&ServiceGroupConfig{GroupID: "private", Enabled: true, Visibility: "custom", AccountIDs: "1"}); err != nil {
		t.Fatal(err)
	}
	ch := addChannel(t, "public", "default", "gpt-live,gpt-closed", 0, 1, common.ChannelStatusEnabled)
	addChannel(t, "private", "private", "private-model", 0, 1, common.ChannelStatusEnabled)
	addChannel(t, "unconfigured", "missing", "hidden-model", 0, 1, common.ChannelStatusEnabled)
	if _, err := SetChannelResourceEnabled(ch.ID, "gpt-closed", false); err != nil {
		t.Fatal(err)
	}
	if err := UpsertModelMeta(&ModelMeta{Model: "metadata-only", Vendor: "Hidden"}); err != nil {
		t.Fatal(err)
	}
	view, err := ReadPublicStatusOverview(t.Context())
	if err != nil || len(view.Models) != 2 || view.Models[0].ID != "gpt-closed" || view.Models[0].Available || !view.Models[1].Available {
		t.Fatalf("catalog: %+v %v", view, err)
	}
	encoded, _ := json.Marshal(view)
	for _, secret := range []string{"private-model", "hidden-model", "metadata-only", "channel_id", "user_id", "base_url", "quota", "sk-public"} {
		if strings.Contains(string(encoded), secret) {
			t.Fatalf("public response exposed %s", secret)
		}
	}
}

func TestPublicStatusCatalogKeepsOnlyValidManagedGrants(t *testing.T) {
	setting.ResetAllToDefaults()
	t.Cleanup(setting.ResetAllToDefaults)
	ch, _, bindings := publicationFixture(t)
	if err := LoadOptionsToSettings(); err != nil {
		t.Fatal(err)
	}
	plan := publicationPlan(t, bindings)
	if _, err := PublishResourceGateway(t.Context(), publicationExpected(&plan.ResourcePublicationContext), bindings, plan.ReviewDigest, 1); err != nil {
		t.Fatal(err)
	}
	if _, err := SetChannelResourcesEnabled(ch.ID, false); err != nil {
		t.Fatal(err)
	}
	view, err := ReadPublicStatusOverview(t.Context())
	if err != nil || len(view.Models) != 2 || view.Models[0].Available {
		t.Fatalf("closed published resources: %+v %v", view, err)
	}
	if err := revokeResourceGatewayChannel(DB, ch.ID); err != nil {
		t.Fatal(err)
	}
	view, err = ReadPublicStatusOverview(t.Context())
	if err != nil || len(view.Models) != 0 {
		t.Fatalf("revoked grants remained public: %+v %v", view, err)
	}
}
