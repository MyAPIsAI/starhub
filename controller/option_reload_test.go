package controller

import (
	"testing"
	"time"

	"starhub/model"
	"starhub/setting"
)

func TestOptionReloadAppliesLocalVersionWithoutClaimingPeerSync(t *testing.T) {
	newTestDB(t)
	if err := model.LoadOptionsToSettings(); err != nil {
		t.Fatal(err)
	}
	before := setting.ModelRatioValue("gpt-4o")
	version, err := model.SaveOption(setting.OptionKeyModelRatio, `{"gpt-4o":123}`, 1)
	if err != nil {
		t.Fatal(err)
	}
	if setting.ModelRatioValue("gpt-4o") != before {
		t.Fatal("database update unexpectedly changed local settings")
	}
	for _, instance := range []model.OptionInstance{{ID: "peer-current", Version: version, LastSeen: time.Now().Unix()}, {ID: "peer-old", Version: version - 1, LastSeen: time.Now().Unix()}} {
		if err := model.DB.Create(&instance).Error; err != nil {
			t.Fatal(err)
		}
	}
	rec := call(ReloadOptions, testRequest{method: "POST", target: "/api/option/reload"})
	var data struct {
		DatabaseVersion int64                  `json:"database_version"`
		LoadedVersion   int64                  `json:"loaded_version"`
		InSync          bool                   `json:"in_sync"`
		Instances       []model.OptionInstance `json:"instances"`
	}
	decodeData(t, rec, &data)
	if rec.Code != 200 || data.DatabaseVersion != version || data.LoadedVersion != version || !data.InSync || len(data.Instances) != 2 || setting.ModelRatioValue("gpt-4o") != 123 {
		t.Fatalf("reload status: %d %s", rec.Code, rec.Body)
	}
	if data.Instances[1].Version != version-1 {
		t.Fatal("reload fabricated an acknowledgment from the lagging peer")
	}
}

func TestOptionReloadRejectsInvalidStoredConfiguration(t *testing.T) {
	newTestDB(t)
	if err := model.LoadOptionsToSettings(); err != nil {
		t.Fatal(err)
	}
	before := setting.ModelRatioValue("gpt-4o")
	loaded := model.LoadedOptionVersion()
	if err := model.UpsertOption(setting.OptionKeyModelRatio, `not-json`); err != nil {
		t.Fatal(err)
	}
	rec := call(ReloadOptions, testRequest{method: "POST", target: "/api/option/reload"})
	if rec.Code != 500 || setting.ModelRatioValue("gpt-4o") != before || model.LoadedOptionVersion() != loaded {
		t.Fatalf("invalid reload changed effective settings: %d %s", rec.Code, rec.Body)
	}
}
