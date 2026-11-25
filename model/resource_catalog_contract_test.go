package model

import (
	"encoding/json"
	"os"
	"testing"

	"starhub/setting"
)

func TestEffectiveCatalogSyncFixture(t *testing.T) {
	newTestDB(t)
	setting.ResetAllToDefaults()
	t.Cleanup(setting.ResetAllToDefaults)
	if err := setting.UpdateOption("group_ratio", `{"default":0.5}`); err != nil {
		t.Fatal(err)
	}
	ch := &Channel{Name: "contract", Key: "test-only-secret", Models: "glm-5.2,glm-5.3", Group: "default", Status: 1, Type: 1, BaseURL: "http://fixture.invalid"}
	if err := InsertChannel(ch); err != nil {
		t.Fatal(err)
	}
	before, err := ReadPlatformCatalog()
	if err != nil {
		t.Fatal(err)
	}
	if err := setting.UpdatePricing(setting.OptionKeyCacheRatio, "0.25"); err != nil {
		t.Fatal(err)
	}
	after, err := ReadPlatformCatalog()
	if err != nil {
		t.Fatal(err)
	}
	stable, err := ReadPlatformCatalog()
	if err != nil {
		t.Fatal(err)
	}
	if after.Revision <= before.Revision || stable.Revision != after.Revision {
		t.Fatal("cache changes or stable revisions lost")
	}
	if path := os.Getenv("STARHUB_CATALOG_FIXTURE"); path != "" {
		raw, err := json.Marshal([]PlatformCatalog{before, after})
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, raw, 0600); err != nil {
			t.Fatal(err)
		}
	}
}
