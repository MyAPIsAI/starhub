package model

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"testing"

	"starhub/common"
	"starhub/setting"
)

type publicationDatabaseState struct {
	Catalog         *ResourceCatalog
	Channels        []Channel
	Abilities       []Ability
	Owners          []ResourceGatewayManagedChannel
	Routes          []ResourceGatewayRoute
	Receipts        []ResourceGatewayPublication
	BindingChanges  []ResourceGatewayBindingChange
	ChannelRevision int64
}

func publicationDatabaseSnapshot(t *testing.T) publicationDatabaseState {
	t.Helper()
	catalog, err := ReadResourceCatalog(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	state := publicationDatabaseState{Catalog: catalog, ChannelRevision: channelRevision(t)}
	for _, query := range []struct {
		value any
		order string
	}{{&state.Channels, "id"}, {&state.Abilities, "channel_id,model,\"group\""}, {&state.Owners, "channel_id"}, {&state.Routes, "channel_id,model,\"group\""}, {&state.Receipts, "id"}, {&state.BindingChanges, "id"}} {
		if err := DB.Order(query.order).Find(query.value).Error; err != nil {
			t.Fatal(err)
		}
	}
	return state
}

func publishReviewedFixture(t *testing.T, bindings []ResourceChannelBinding) (*ResourcePublicationPreview, *ResourcePublicationAcknowledgement) {
	t.Helper()
	plan := publicationPlan(t, bindings)
	ack, err := PublishResourceGateway(context.Background(), publicationExpected(&plan.ResourcePublicationContext), bindings, plan.ReviewDigest, 7)
	if err != nil {
		t.Fatal(err)
	}
	return plan, ack
}

func TestResourcePublicationCommitsOnlyReviewedPairsAndDurableReceipt(t *testing.T) {
	ch, _, bindings := publicationFixture(t)
	before := publicationDatabaseSnapshot(t)
	plan, ack := publishReviewedFixture(t, bindings)
	if !ack.Published || ack.PublicationID == "" || ack.ReviewDigest != plan.ReviewDigest || ack.RouteCount != 2 || ack.BindingCount != 1 || ack.ResourceRevision != plan.ResourceRevision+1 || ack.ChannelRevision != plan.ChannelRevision+1 || ack.OptionVersion != plan.OptionVersion {
		t.Fatalf("acknowledgement lost committed state: %+v", ack)
	}
	after := publicationDatabaseSnapshot(t)
	if !after.Catalog.AffectsGatewayRouting || after.Catalog.Revision != ack.ResourceRevision || after.ChannelRevision != ack.ChannelRevision || !reflect.DeepEqual(before.Catalog.Snapshot, after.Catalog.Snapshot) || !reflect.DeepEqual(before.Catalog.Placements, after.Catalog.Placements) || !reflect.DeepEqual(before.Channels, after.Channels) {
		t.Fatalf("publication changed inventory/channel identity: %+v", after)
	}
	if len(after.Abilities) != 2 || len(after.Routes) != 2 || len(after.Owners) != 1 || !after.Owners[0].Active || len(after.Receipts) != 1 || after.Owners[0].PublicationID != after.Receipts[0].ID {
		t.Fatalf("partial publication: %+v", after)
	}
	var recorded []ResourcePublicationRoute
	if err := json.Unmarshal([]byte(after.Receipts[0].Routes), &recorded); err != nil || !reflect.DeepEqual(recorded, plan.ProposedRoutes) || after.Receipts[0].ActorID != 7 {
		t.Fatalf("receipt lost reviewed constraints: %+v %v", recorded, err)
	}
	if strconv.FormatInt(after.Receipts[0].ID, 10) != ack.PublicationID {
		t.Fatal("receipt ID not exact")
	}
	assertGatewayPairs(t, ch.ID, true)
	if err := RebuildAbilities(ch); err != nil {
		t.Fatal(err)
	}
	assertGatewayPairs(t, ch.ID, true)
	for _, table := range []any{&Wallet{}, &LedgerEntry{}, &Token{}, &Log{}} {
		var count int64
		if err := DB.Model(table).Count(&count).Error; err != nil || count != 0 {
			t.Fatalf("publishing created account/fund/usage data: %d %v", count, err)
		}
	}
}

func TestResourcePublicationRevalidatesAllDependenciesAndReviewDigest(t *testing.T) {
	for _, change := range []string{"resource", "channel", "options", "group-identity", "group-metadata", "ownership", "digest"} {
		t.Run(change, func(t *testing.T) {
			ch, _, bindings := publicationFixture(t)
			if change == "group-metadata" {
				if err := UpsertServiceGroup(&ServiceGroupConfig{GroupID: "default", Name: "Before", Enabled: true, Visibility: "common"}); err != nil {
					t.Fatal(err)
				}
			}
			plan := publicationPlan(t, bindings)
			switch change {
			case "resource":
				if _, err := SaveResourceRegion(context.Background(), plan.ResourceRevision, ResourceRegion{ID: "region", Name: "Changed"}); err != nil {
					t.Fatal(err)
				}
			case "channel":
				ch.Name = "Changed"
				if err := UpdateChannel(ch); err != nil {
					t.Fatal(err)
				}
			case "options":
				if _, err := SaveOption(setting.OptionKeyCacheRatio, "0.8", 7); err != nil {
					t.Fatal(err)
				}
			case "group-identity":
				if err := UpsertServiceGroup(&ServiceGroupConfig{GroupID: "default", Name: "New", Enabled: true, Visibility: "common"}); err != nil {
					t.Fatal(err)
				}
			case "group-metadata":
				group, err := GetServiceGroup("default")
				if err != nil {
					t.Fatal(err)
				}
				group.Name = "After"
				if err := UpsertServiceGroup(group); err != nil {
					t.Fatal(err)
				}
			case "ownership":
				if err := DB.Create(&ResourceGatewayManagedChannel{ChannelID: ch.ID, SourceChannelID: bindings[0].SourceChannelID, Profile: ResourceGatewayTextProfile, Active: false}).Error; err != nil {
					t.Fatal(err)
				}
			case "digest":
				plan.ReviewDigest = strings.Repeat("a", 64)
			}
			before := publicationDatabaseSnapshot(t)
			if _, err := PublishResourceGateway(context.Background(), publicationExpected(&plan.ResourcePublicationContext), bindings, plan.ReviewDigest, 7); !errors.Is(err, ErrResourceConflict) {
				t.Fatalf("stale %s accepted: %v", change, err)
			}
			if after := publicationDatabaseSnapshot(t); !reflect.DeepEqual(before, after) {
				t.Fatalf("stale %s changed state", change)
			}
		})
	}
}

func TestResourcePublicationRejectsBlockedPlansAndInvalidInputs(t *testing.T) {
	_, _, bindings := publicationFixture(t)
	catalog, err := ReadResourceCatalog(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	placement := catalog.Placements["resource-1"]
	modelID := "unsupported-alias"
	placement.ModelID = &modelID
	if _, err := SaveResourcePlacement(context.Background(), catalog.Revision, "resource-1", placement); err != nil {
		t.Fatal(err)
	}
	plan := publicationPlan(t, bindings)
	if plan.Ready {
		t.Fatal("test did not build a blocked plan")
	}
	before := publicationDatabaseSnapshot(t)
	if _, err := PublishResourceGateway(context.Background(), publicationExpected(&plan.ResourcePublicationContext), bindings, plan.ReviewDigest, 7); !errors.Is(err, ErrInvalidResource) {
		t.Fatalf("blocked plan published: %v", err)
	}
	for _, digest := range []string{"", strings.Repeat("g", 64), strings.Repeat("a", 63)} {
		if _, err := PublishResourceGateway(context.Background(), publicationExpected(&plan.ResourcePublicationContext), bindings, digest, 7); !errors.Is(err, ErrInvalidResource) {
			t.Fatalf("invalid digest accepted: %v", err)
		}
	}
	if _, err := PublishResourceGateway(context.Background(), publicationExpected(&plan.ResourcePublicationContext), bindings, plan.ReviewDigest, 0); !errors.Is(err, ErrInvalidResource) {
		t.Fatalf("anonymous publication: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := PublishResourceGateway(ctx, publicationExpected(&plan.ResourcePublicationContext), bindings, plan.ReviewDigest, 7); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled publication: %v", err)
	}
	if after := publicationDatabaseSnapshot(t); !reflect.DeepEqual(before, after) {
		t.Fatal("invalid/blocked/cancelled publication changed state")
	}
}

func TestResourcePublicationConcurrentWritersHaveOneWinner(t *testing.T) {
	_, _, bindings := publicationFixture(t)
	plan := publicationPlan(t, bindings)
	var workers sync.WaitGroup
	failures := make(chan error, 8)
	for i := 0; i < 8; i++ {
		workers.Add(1)
		go func() {
			defer workers.Done()
			_, err := PublishResourceGateway(context.Background(), publicationExpected(&plan.ResourcePublicationContext), bindings, plan.ReviewDigest, 7)
			failures <- err
		}()
	}
	workers.Wait()
	close(failures)
	winners := 0
	for err := range failures {
		if err == nil {
			winners++
		} else if !errors.Is(err, ErrResourceConflict) {
			t.Fatal(err)
		}
	}
	after := publicationDatabaseSnapshot(t)
	if winners != 1 || len(after.Receipts) != 1 || len(after.Routes) != 2 || len(after.Owners) != 1 || after.Catalog.Revision != plan.ResourceRevision+1 || after.ChannelRevision != plan.ChannelRevision+1 {
		t.Fatalf("concurrent partial/duplicate publication: winners=%d state=%+v", winners, after)
	}
}

func TestResourcePublicationRollsBackEveryWriteStage(t *testing.T) {
	for _, stage := range []string{"receipt", "owner", "route", "ability", "channel-version", "resource-version"} {
		t.Run(stage, func(t *testing.T) {
			_, _, bindings := publicationFixture(t)
			plan := publicationPlan(t, bindings)
			before := publicationDatabaseSnapshot(t)
			trigger := ""
			switch stage {
			case "receipt":
				trigger = `CREATE TRIGGER fail_publication BEFORE INSERT ON resource_gateway_publications BEGIN SELECT RAISE(ABORT,'injected'); END`
			case "owner":
				trigger = `CREATE TRIGGER fail_publication BEFORE INSERT ON resource_gateway_managed_channels BEGIN SELECT RAISE(ABORT,'injected'); END`
			case "route":
				trigger = `CREATE TRIGGER fail_publication BEFORE INSERT ON resource_gateway_routes BEGIN SELECT RAISE(ABORT,'injected'); END`
			case "ability":
				trigger = `CREATE TRIGGER fail_publication BEFORE INSERT ON abilities BEGIN SELECT RAISE(ABORT,'injected'); END`
			case "channel-version":
				trigger = `CREATE TRIGGER fail_publication BEFORE UPDATE ON channel_configuration_states BEGIN SELECT RAISE(ABORT,'injected'); END`
			case "resource-version":
				trigger = `CREATE TRIGGER fail_publication BEFORE UPDATE OF revision ON resource_catalog_records WHEN NEW.revision <> OLD.revision BEGIN SELECT RAISE(ABORT,'injected'); END`
			}
			if err := DB.Exec(trigger).Error; err != nil {
				t.Fatal(err)
			}
			if ack, err := PublishResourceGateway(context.Background(), publicationExpected(&plan.ResourcePublicationContext), bindings, plan.ReviewDigest, 7); err == nil || ack != nil {
				t.Fatalf("failed %s acknowledged: %+v %v", stage, ack, err)
			}
			if after := publicationDatabaseSnapshot(t); !reflect.DeepEqual(before, after) {
				t.Fatalf("failed %s left partial state", stage)
			}
		})
	}
}

func TestResourcePublicationSurvivesRestartAndReceiptRemainsHistorical(t *testing.T) {
	path := filepath.Join(t.TempDir(), "publication.db")
	if err := InitDB(path); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = CloseDB() })
	if err := SeedOptions(); err != nil {
		t.Fatal(err)
	}
	if _, err := SaveOption(setting.OptionKeyGroupRatio, `{"default":1}`, 7); err != nil {
		t.Fatal(err)
	}
	ch := addChannel(t, "gateway", "default", "alpha", 0, 1, common.ChannelStatusEnabled)
	snapshot := resourceFixture()
	snapshot.Instances[0].UpstreamModel = "alpha"
	snapshot.Instances[0].CanonicalModel = "alpha"
	state, err := SaveResourceSnapshot(context.Background(), 0, snapshot)
	if err != nil {
		t.Fatal(err)
	}
	modelID := "alpha"
	if _, err = SaveResourcePlacement(context.Background(), state.Revision, "resource-1", ResourcePlacement{ModelID: &modelID, GroupIDs: []string{"default"}, Enabled: true}); err != nil {
		t.Fatal(err)
	}
	bindings := []ResourceChannelBinding{{SourceChannelID: "supplier", GatewayChannelID: strconv.FormatInt(ch.ID, 10)}}
	plan, ack := publishReviewedFixture(t, bindings)
	if err := CloseDB(); err != nil {
		t.Fatal(err)
	}
	if err := InitDB(path); err != nil {
		t.Fatal(err)
	}
	receipt, err := ReadResourcePublicationReceipt(context.Background(), plan.ReviewDigest)
	if err != nil || !receipt.Historical || !receipt.Committed || receipt.PublicationID != ack.PublicationID || !reflect.DeepEqual(receipt.Routes, plan.ProposedRoutes) {
		t.Fatalf("receipt not durable: %+v %v", receipt, err)
	}
	if got, err := GetSatisfiedChannel("default", "alpha"); err != nil || got.ID != ch.ID {
		t.Fatalf("publication not durable: %+v %v", got, err)
	}
	state, err = ReadResourceCatalog(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	placement := state.Placements["resource-1"]
	placement.Enabled = false
	if _, err := SaveResourcePlacement(context.Background(), state.Revision, "resource-1", placement); err != nil {
		t.Fatal(err)
	}
	if _, err := GetSatisfiedChannel("default", "alpha"); !errors.Is(err, ErrNoChannel) {
		t.Fatalf("resource revocation failed: %v", err)
	}
	again, err := ReadResourcePublicationReceipt(context.Background(), plan.ReviewDigest)
	if err != nil || !reflect.DeepEqual(receipt, again) {
		t.Fatal("revocation rewrote historical receipt")
	}
}

func TestResourcePublicationReceiptRejectsMissingAndCorruptData(t *testing.T) {
	_, _, bindings := publicationFixture(t)
	plan, _ := publishReviewedFixture(t, bindings)
	if _, err := ReadResourcePublicationReceipt(context.Background(), "invalid"); !errors.Is(err, ErrInvalidResource) {
		t.Fatalf("invalid receipt lookup: %v", err)
	}
	if _, err := ReadResourcePublicationReceipt(context.Background(), strings.Repeat("0", 64)); !errors.Is(err, ErrResourcePublicationNotFound) {
		t.Fatalf("missing receipt lookup: %v", err)
	}
	if err := DB.Model(&ResourceGatewayPublication{}).Where("review_digest = ?", plan.ReviewDigest).Update("routes", `[{"model":"fake"}]`).Error; err != nil {
		t.Fatal(err)
	}
	if receipt, err := ReadResourcePublicationReceipt(context.Background(), plan.ReviewDigest); err == nil || receipt != nil {
		t.Fatal("corrupt receipt synthesized route facts")
	}
}

func TestResourcePublicationExhaustedVersionsAreNotPublishable(t *testing.T) {
	for _, kind := range []string{"resource", "channel"} {
		t.Run(kind, func(t *testing.T) {
			_, _, bindings := publicationFixture(t)
			if kind == "resource" {
				if err := DB.Model(&ResourceCatalogRecord{}).Where("id = 1").UpdateColumn("revision", maxResourceRevision).Error; err != nil {
					t.Fatal(err)
				}
			} else {
				if err := DB.Model(&ChannelConfigurationState{}).Where("id = 1").UpdateColumn("revision", maxResourceRevision).Error; err != nil {
					t.Fatal(err)
				}
			}
			plan := publicationPlan(t, bindings)
			if plan.Ready || !publicationIssue(plan, "publication_revision_exhausted") {
				t.Fatal("preview claimed exhausted versions were publishable")
			}
			before := publicationDatabaseSnapshot(t)
			if _, err := PublishResourceGateway(context.Background(), publicationExpected(&plan.ResourcePublicationContext), bindings, plan.ReviewDigest, 7); !errors.Is(err, ErrInvalidResource) {
				t.Fatalf("exhausted version published: %v", err)
			}
			if after := publicationDatabaseSnapshot(t); !reflect.DeepEqual(before, after) {
				t.Fatal("exhausted version changed publication")
			}
		})
	}
}

func TestResourcePublicationSourceBindingIndexAllowsOnlyReleasedDuplicates(t *testing.T) {
	_, _, bindings := publicationFixture(t)
	publishReviewedFixture(t, bindings)
	if err := DB.Create(&ResourceGatewayManagedChannel{ChannelID: 9000, SourceChannelID: bindings[0].SourceChannelID, Profile: ResourceGatewayTextProfile, Active: false}).Error; err == nil {
		t.Fatal("database accepted duplicate source binding")
	}
	if err := DB.Create(&[]ResourceGatewayManagedChannel{{ChannelID: 9001, SourceChannelID: "", Profile: ResourceGatewayTextProfile, Active: false}, {ChannelID: 9002, SourceChannelID: "", Profile: ResourceGatewayTextProfile, Active: false}}).Error; err != nil {
		t.Fatalf("released tombstones cannot coexist: %v", err)
	}
}
