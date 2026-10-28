package model

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strconv"
	"strings"
	"testing"

	"gorm.io/gorm"
)

func TestResourceBindingStateReturnsNoResultWhenReadCommitIsCancelled(t *testing.T) {
	publicationFixture(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	const callback = "cancel_binding_state_after_owner_read"
	if err := DB.Callback().Query().After("gorm:after_query").Register(callback, func(tx *gorm.DB) {
		if tx.Statement.Table == "resource_gateway_managed_channels" {
			cancel()
		}
	}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { DB.Callback().Query().Remove(callback) })
	if state, err := ReadResourceBindingState(ctx, 1, 20); err == nil || state != nil {
		t.Fatalf("cancelled read returned usable state: %+v %v", state, err)
	}
}

func TestResourceBindingStateDoesNotConfuseConfiguredWithRouteEligible(t *testing.T) {
	for _, change := range []string{"profile", "capabilities", "context", "type", "disabled", "unpriced", "cross-pair"} {
		t.Run(change, func(t *testing.T) {
			ch, _, bindings := publicationFixture(t)
			publishReviewedFixture(t, bindings)
			var err error
			want := int64(0)
			switch change {
			case "profile":
				err = DB.Model(&ResourceGatewayManagedChannel{}).Where("channel_id = ?", ch.ID).Update("profile", "unknown").Error
			case "capabilities":
				err = DB.Model(&ResourceGatewayRoute{}).Where("channel_id = ?", ch.ID).Update("capabilities", `["vision"]`).Error
			case "context":
				err = DB.Model(&ResourceGatewayRoute{}).Where("channel_id = ?", ch.ID).Update("context_length", maxResourceRevision+1).Error
			case "type":
				err = DB.Model(&Channel{}).Where("id = ?", ch.ID).Update("type", 99).Error
			case "disabled":
				err = DB.Model(&Channel{}).Where("id = ?", ch.ID).Update("status", 2).Error
			case "unpriced":
				err = DB.Model(&Option{}).Where("\"key\" = ?", "group_ratio").Update("value", `{"default":0,"vip":0}`).Error
			case "cross-pair":
				err = DB.Create(&Ability{ChannelID: ch.ID, Model: "alpha", Group: "vip", Enabled: true}).Error
				want = 2
			}
			if err != nil {
				t.Fatal(err)
			}
			state, err := ReadResourceBindingState(context.Background(), 1, 20)
			if err != nil || len(state.Bindings) != 1 || !state.Bindings[0].ConfiguredActive || state.Bindings[0].StoredPairCount != 2 || state.Bindings[0].RouteEligiblePairCount != want {
				t.Fatalf("%s bypassed current gate: %+v %v", change, state, err)
			}
		})
	}
}

func TestResourceBindingStateTracksCurrentRouteGatesWithoutMutation(t *testing.T) {
	ch, _, bindings := publicationFixture(t)
	_, ack := publishReviewedFixture(t, bindings)
	before := publicationDatabaseSnapshot(t)
	state, err := ReadResourceBindingState(context.Background(), 1, 20)
	if err != nil || len(state.Bindings) != 1 {
		t.Fatalf("state: %+v %v", state, err)
	}
	row := state.Bindings[0]
	if state.Schema != ResourceBindingStateSchema || !state.ReadOnly || state.ResourceRevision != ack.ResourceRevision || state.ChannelRevision != ack.ChannelRevision || state.OptionVersion != ack.OptionVersion || state.Total != 1 || state.Page != 1 || state.PageSize != 20 || row.GatewayChannelID != strconv.FormatInt(ch.ID, 10) || row.GatewayName != ch.Name || !row.GatewayPresent || !row.SourcePresent || !row.SourceEnabled || row.SourceChannelID != bindings[0].SourceChannelID || !row.ConfiguredActive || row.Profile != ResourceGatewayTextProfile || row.PublicationID != ack.PublicationID || row.StoredPairCount != 2 || row.RouteEligiblePairCount != 2 {
		t.Fatalf("lost binding state: %+v", state)
	}
	encoded, _ := json.Marshal(state)
	for _, secret := range []string{ch.Key, ch.BaseURL, "private.invalid", `"key"`, `"base_url"`} {
		if strings.Contains(string(encoded), secret) {
			t.Fatalf("secret exposed: %q", secret)
		}
	}
	if after := publicationDatabaseSnapshot(t); !reflect.DeepEqual(before, after) {
		t.Fatal("reading state mutated publication")
	}
	if err := DB.Model(&Ability{}).Where("channel_id = ? AND model = ?", ch.ID, "alpha").Update("enabled", false).Error; err != nil {
		t.Fatal(err)
	}
	state, err = ReadResourceBindingState(context.Background(), 1, 20)
	if err != nil || state.Bindings[0].RouteEligiblePairCount != 1 || state.Bindings[0].StoredPairCount != 2 || !state.Bindings[0].ConfiguredActive {
		t.Fatalf("disabled ability: %+v %v", state, err)
	}
	if err := UpsertServiceGroup(&ServiceGroupConfig{GroupID: "vip", Name: "New identity", Enabled: true, Visibility: "common"}); err != nil {
		t.Fatal(err)
	}
	state, err = ReadResourceBindingState(context.Background(), 1, 20)
	if err != nil || state.Bindings[0].RouteEligiblePairCount != 0 || !state.Bindings[0].ConfiguredActive {
		t.Fatalf("group identity bypass: %+v %v", state, err)
	}
	if err := DeleteChannel(ch.ID); err != nil {
		t.Fatal(err)
	}
	state, err = ReadResourceBindingState(context.Background(), 1, 20)
	if err != nil || len(state.Bindings) != 1 {
		t.Fatalf("deleted gateway: %+v %v", state, err)
	}
	row = state.Bindings[0]
	if row.GatewayPresent || row.GatewayName != "" || row.ConfiguredActive || row.StoredPairCount != 0 || row.RouteEligiblePairCount != 0 || row.SourceChannelID != bindings[0].SourceChannelID || row.PublicationID != ack.PublicationID {
		t.Fatalf("reserved orphan disappeared: %+v", row)
	}
}

func TestResourceBindingStatePaginationAndReleasedMarkersPreserveExactIDs(t *testing.T) {
	publicationFixture(t)
	const first int64 = 9007199254740993
	for i := int64(0); i < 23; i++ {
		owner := ResourceGatewayManagedChannel{ChannelID: first + i, SourceChannelID: fmt.Sprintf("missing-source-%d", i), PublicationID: first + i}
		if i == 0 {
			owner.SourceChannelID = ""
		}
		if err := DB.Create(&owner).Error; err != nil {
			t.Fatal(err)
		}
	}
	before := publicationDatabaseSnapshot(t)
	seen := map[string]bool{}
	for page := 1; page <= 4; page++ {
		state, err := ReadResourceBindingState(context.Background(), page, 10)
		if err != nil || state.Total != 23 {
			t.Fatalf("page%d: %+v %v", page, state, err)
		}
		want := 10
		if page == 3 {
			want = 3
		}
		if page == 4 {
			want = 0
		}
		if len(state.Bindings) != want {
			t.Fatalf("page count%d", len(state.Bindings))
		}
		for _, row := range state.Bindings {
			if seen[row.GatewayChannelID] || row.GatewayPresent || row.SourcePresent || row.SourceEnabled || row.PublicationID != row.GatewayChannelID || row.RouteEligiblePairCount != 0 {
				t.Fatalf("lossy or duplicate orphan: %+v", row)
			}
			seen[row.GatewayChannelID] = true
		}
		if page == 1 && state.Bindings[0].SourceChannelID != "" {
			t.Fatal("released tombstone hidden")
		}
	}
	if !seen["9007199254740993"] || len(seen) != 23 {
		t.Fatal("exact identifiers lost")
	}
	if !reflect.DeepEqual(before, publicationDatabaseSnapshot(t)) {
		t.Fatal("paginated read changed state")
	}
}

func TestResourceBindingStateRejectsInvalidAndUnavailableState(t *testing.T) {
	for _, failure := range []string{"owner", "routes", "abilities", "catalog", "channel-version", "option-version", "corrupt-owner", "corrupt-revision", "cancelled"} {
		t.Run(failure, func(t *testing.T) {
			_, _, bindings := publicationFixture(t)
			publishReviewedFixture(t, bindings)
			ctx := context.Background()
			var err error
			switch failure {
			case "owner":
				err = DB.Migrator().DropTable(&ResourceGatewayManagedChannel{})
			case "routes":
				err = DB.Migrator().DropTable(&ResourceGatewayRoute{})
			case "abilities":
				err = DB.Migrator().DropTable(&Ability{})
			case "catalog":
				err = DB.Migrator().DropTable(&ResourceCatalogRecord{})
			case "channel-version":
				err = DB.Migrator().DropTable(&ChannelConfigurationState{})
			case "option-version":
				err = DB.Migrator().DropTable(&OptionRevision{})
			case "corrupt-owner":
				err = DB.Model(&ResourceGatewayManagedChannel{}).Where("1 = 1").Update("publication_id", -1).Error
			case "corrupt-revision":
				err = DB.Model(&ChannelConfigurationState{}).Where("id = 1").Update("revision", -1).Error
			case "cancelled":
				var cancel context.CancelFunc
				ctx, cancel = context.WithCancel(ctx)
				cancel()
			}
			if err != nil {
				t.Fatal(err)
			}
			if state, err := ReadResourceBindingState(ctx, 1, 20); err == nil || state != nil {
				t.Fatalf("partial unavailable state: %+v %v", state, err)
			}
		})
	}
	for _, input := range [][2]int{{0, 20}, {-1, 20}, {1000001, 20}, {1, 0}, {1, 101}} {
		if state, err := ReadResourceBindingState(context.Background(), input[0], input[1]); !errors.Is(err, ErrInvalidResource) || state != nil {
			t.Fatalf("invalid pagination: %+v %v", state, err)
		}
	}
}
