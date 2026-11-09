package model

import (
	"testing"
	"time"

	"starhub/setting"
)

func TestUIAC18CatalogGraceRecoveryAndRestart(t *testing.T) {
	path := groupConfigurationDB(t)
	if _, err := SaveOption(setting.OptionKeyGroupRatio, `{"public":0.6,"private":0.1}`, 0); err != nil {
		t.Fatal(err)
	}
	if err := UpsertServiceGroup(&ServiceGroupConfig{GroupID: "private", Enabled: true, Visibility: "custom", AccountIDs: "1"}); err != nil {
		t.Fatal(err)
	}
	ch := &Channel{Name: "Catalog grace fixture", Type: 1, Status: 1, Models: "grace-model", Group: "public"}
	if err := InsertChannel(ch); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 11, 9, 0, 0, 0, 0, time.UTC)
	read := func(at time.Time) CatalogAvailability {
		t.Helper()
		s, err := ReadPublicCatalogState(t.Context(), at)
		if err != nil {
			t.Fatal(err)
		}
		if len(s.Groups) != 1 || s.Groups[0].GroupID != "public" {
			t.Fatalf("private group leaked: %+v", s.Groups)
		}
		return s.Availability["grace-model"]["public"]
	}
	set := func(on bool) {
		t.Helper()
		if err := DB.Model(&Ability{}).Where("channel_id = ?", ch.ID).Update("enabled", on).Error; err != nil {
			t.Fatal(err)
		}
	}
	if s := read(now); !s.Available || !s.Visible(now) {
		t.Fatal(s)
	}
	set(false)
	start := now.Add(time.Second)
	read(start)
	if s := read(start.Add(5 * time.Minute)); s.Available || !s.Visible(start.Add(5*time.Minute)) {
		t.Fatal(s)
	}
	set(true)
	if s := read(start.Add(5*time.Minute + time.Second)); !s.Available || s.UnavailableSince != 0 {
		t.Fatal(s)
	}
	set(false)
	start = start.Add(6 * time.Minute)
	read(start)
	if err := CloseDB(); err != nil {
		t.Fatal(err)
	}
	if err := InitDB(path); err != nil {
		t.Fatal(err)
	}
	if s := read(start.Add(PublicCatalogGrace - time.Millisecond)); !s.Visible(start.Add(PublicCatalogGrace - time.Millisecond)) {
		t.Fatal(s)
	}
	if s := read(start.Add(PublicCatalogGrace)); s.Visible(start.Add(PublicCatalogGrace)) {
		t.Fatal(s)
	}
	if s := read(start.Add(12 * time.Minute)); s.Visible(start.Add(12 * time.Minute)) {
		t.Fatal(s)
	}
	set(true)
	if s := read(start.Add(12*time.Minute + time.Second)); !s.Available || !s.Visible(start.Add(12*time.Minute+time.Second)) {
		t.Fatal(s)
	}
}

func TestPublicCatalogNewUnavailableGroupGetsNoGrace(t *testing.T) {
	groupConfigurationDB(t)
	if _, err := SaveOption(setting.OptionKeyGroupRatio, `{"zero":0}`, 0); err != nil {
		t.Fatal(err)
	}
	if err := UpsertModelMeta(&ModelMeta{Model: "unserved", Tags: "text"}); err != nil {
		t.Fatal(err)
	}
	at := time.Now()
	s, err := ReadPublicCatalogState(t.Context(), at)
	if err != nil {
		t.Fatal(err)
	}
	if v := s.Availability["unserved"]["zero"]; v.Visible(at) || v.EverAvailable {
		t.Fatal(v)
	}
}
