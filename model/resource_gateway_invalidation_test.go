package model

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"starhub/common"
)

func resourceGatewayInventoryFixture(t *testing.T) (*Channel, *ResourceCatalog) {
	t.Helper()
	newTestDB(t)
	snapshot := resourceFixture()
	snapshot.Instances[0].CanonicalModel = "alpha"
	snapshot.Instances[0].UpstreamModel = "alpha"
	snapshot.Instances[0].Capabilities = []string{"text", "code"}
	beta := snapshot.Instances[0]
	beta.ID = "resource-2"
	beta.CanonicalModel = "beta"
	beta.UpstreamModel = "beta"
	snapshot.Instances = append(snapshot.Instances, beta)
	catalog, err := SaveResourceSnapshot(context.Background(), 0, snapshot)
	if err != nil {
		t.Fatal(err)
	}
	for i, resource := range snapshot.Instances {
		group := "default"
		if i == 1 {
			group = "vip"
		}
		catalog, err = SaveResourcePlacement(context.Background(), catalog.Revision, resource.ID, ResourcePlacement{ModelID: &resource.CanonicalModel, GroupIDs: []string{group}, Enabled: true})
		if err != nil {
			t.Fatal(err)
		}
	}
	ch := addChannel(t, "managed", "default,vip", "alpha,beta", 0, 1, common.ChannelStatusEnabled)
	managedGatewayFixture(t, ch, true, ResourceGatewayRoute{Model: "alpha", Group: "default"}, ResourceGatewayRoute{Model: "beta", Group: "vip"})
	if err := DB.Model(&ResourceGatewayManagedChannel{}).Where("channel_id = ?", ch.ID).Update("source_channel_id", "supplier").Error; err != nil {
		t.Fatal(err)
	}
	catalog, err = ReadResourceCatalog(context.Background())
	if err != nil || !catalog.AffectsGatewayRouting {
		t.Fatalf("managed effect not declared: %+v %v", catalog, err)
	}
	return ch, catalog
}

func TestResourceGatewayInventoryChangesRevokeWithoutAutomaticRestoration(t *testing.T) {
	for _, change := range []string{"upstream", "canonical", "source-binding", "capabilities", "protocols", "context", "unavailable", "retired", "removed", "source-disabled", "source-removed", "provider"} {
		t.Run(change, func(t *testing.T) {
			ch, catalog := resourceGatewayInventoryFixture(t)
			before := channelRevision(t)
			snapshot := *catalog.Snapshot
			snapshot.Sequence++
			switch change {
			case "upstream":
				snapshot.Instances[0].UpstreamModel = "other"
			case "canonical":
				snapshot.Instances[0].CanonicalModel = "other"
			case "source-binding":
				snapshot.Channels = append(snapshot.Channels, ResourceChannel{ID: "other", Name: "Other", Enabled: true})
				snapshot.Instances[0].ChannelID = "other"
			case "capabilities":
				snapshot.Instances[0].Capabilities = []string{"text"}
			case "protocols":
				snapshot.Instances[0].Protocols = []string{"Anthropic"}
			case "context":
				snapshot.Instances[0].ContextLength = 1000
			case "unavailable":
				snapshot.Instances[0].Status = "unavailable"
			case "retired":
				snapshot.Instances[0].Status = "retired"
			case "removed":
				snapshot.Instances = snapshot.Instances[1:]
			case "source-disabled":
				snapshot.Channels[0].Enabled = false
			case "source-removed":
				snapshot.Channels = []ResourceChannel{}
				snapshot.Instances = []ResourceInstance{}
			case "provider":
				snapshot.Channels[0].Provider = "Changed provider"
			}
			state, err := SaveResourceSnapshot(context.Background(), catalog.Revision, snapshot)
			if err != nil {
				t.Fatal(err)
			}
			if channelRevision(t) != before+1 {
				t.Fatal("revocation did not advance channel revision exactly once")
			}
			assertGatewayPairs(t, ch.ID, false)
			var marker ResourceGatewayManagedChannel
			if err := DB.First(&marker, "channel_id = ?", ch.ID).Error; err != nil || marker.Active {
				t.Fatalf("revocation marker lost: %+v %v", marker, err)
			}
			// Restoring source data and re-enabling placements is never publication.
			original := resourceFixture()
			original.Sequence = 3
			original.Instances[0].CanonicalModel = "alpha"
			original.Instances[0].UpstreamModel = "alpha"
			state, err = SaveResourceSnapshot(context.Background(), state.Revision, original)
			if err != nil {
				t.Fatal(err)
			}
			modelID := "alpha"
			if _, err := SaveResourcePlacement(context.Background(), state.Revision, "resource-1", ResourcePlacement{ModelID: &modelID, GroupIDs: []string{"default"}, Enabled: true}); err != nil {
				t.Fatal(err)
			}
			if err := RebuildAbilities(ch); err != nil {
				t.Fatal(err)
			}
			assertGatewayPairs(t, ch.ID, false)
		})
	}
}

func TestResourceGatewayPlacementChangesRevokeAndProtectUnrelatedChannels(t *testing.T) {
	for _, change := range []string{"disable", "groups", "model"} {
		t.Run(change, func(t *testing.T) {
			ch, catalog := resourceGatewayInventoryFixture(t)
			other := addChannel(t, "other", "default", "unrelated", 0, 1, common.ChannelStatusEnabled)
			if err := DB.Create(&ResourceGatewayManagedChannel{ChannelID: other.ID, SourceChannelID: "unrelated", Active: true}).Error; err != nil {
				t.Fatal(err)
			}
			if err := DB.Create(&ResourceGatewayRoute{ChannelID: other.ID, Model: "unrelated", Group: "default"}).Error; err != nil {
				t.Fatal(err)
			}
			before := channelRevision(t)
			placement := catalog.Placements["resource-1"]
			switch change {
			case "disable":
				placement.Enabled = false
			case "groups":
				placement.GroupIDs = []string{"vip"}
			case "model":
				value := "different"
				placement.ModelID = &value
			}
			if _, err := SaveResourcePlacement(context.Background(), catalog.Revision, "resource-1", placement); err != nil {
				t.Fatal(err)
			}
			if channelRevision(t) != before+1 {
				t.Fatal("placement revocation revision not advanced once")
			}
			assertGatewayPairsWithoutDiscovery(t, ch.ID)
			if got, err := GetSatisfiedChannel("default", "unrelated"); err != nil || got.ID != other.ID {
				t.Fatalf("unrelated source was revoked: %+v %v", got, err)
			}
		})
	}
}

func assertGatewayPairsWithoutDiscovery(t *testing.T, channelID int64) {
	t.Helper()
	for _, pair := range []struct{ model, group string }{{"alpha", "default"}, {"beta", "vip"}} {
		if _, err := GetSatisfiedChannelPreferred(pair.group, pair.model, nil, channelID); !errors.Is(err, ErrNoChannel) {
			t.Fatalf("revoked pair still selectable: %v", err)
		}
	}
}

func TestResourceGatewayNonRoutingMetadataAndNoopPlacementRetainPublishedPairs(t *testing.T) {
	ch, catalog := resourceGatewayInventoryFixture(t)
	before := channelRevision(t)
	snapshot := *catalog.Snapshot
	snapshot.Sequence++
	snapshot.Source = "Metadata refresh"
	snapshot.Channels[0].Name = "New display name"
	snapshot.Instances[0].Capabilities = []string{"code", "text"}
	snapshot.Instances[0].Region = "new-region"
	snapshot.Instances[0].LatencyMS = 20
	state, err := SaveResourceSnapshot(context.Background(), catalog.Revision, snapshot)
	if err != nil {
		t.Fatal(err)
	}
	state, err = SaveResourcePlacement(context.Background(), state.Revision, "resource-1", state.Placements["resource-1"])
	if err != nil {
		t.Fatal(err)
	}
	if _, err := SaveResourceRegion(context.Background(), state.Revision, ResourceRegion{ID: "new-region", Name: "Display region"}); err != nil {
		t.Fatal(err)
	}
	if channelRevision(t) != before {
		t.Fatal("non-routing metadata revoked publication")
	}
	assertGatewayPairs(t, ch.ID, true)
}

func TestResourceGatewayRevocationAndInventoryRollbackTogether(t *testing.T) {
	ch, catalog := resourceGatewayInventoryFixture(t)
	before := channelRevision(t)
	if err := DB.Exec(`CREATE TRIGGER reject_resource_snapshot BEFORE UPDATE OF snapshot ON resource_catalog_records BEGIN SELECT RAISE(ABORT, 'injected resource failure'); END`).Error; err != nil {
		t.Fatal(err)
	}
	snapshot := *catalog.Snapshot
	snapshot.Instances = append([]ResourceInstance(nil), snapshot.Instances...)
	snapshot.Sequence++
	snapshot.Instances[0].Status = "retired"
	if _, err := SaveResourceSnapshot(context.Background(), catalog.Revision, snapshot); err == nil {
		t.Fatal("injected failure not returned")
	}
	after, err := ReadResourceCatalog(context.Background())
	if err != nil || !reflect.DeepEqual(catalog, after) || channelRevision(t) != before {
		t.Fatalf("failed inventory update retained revocation: %+v %v", after, err)
	}
	assertGatewayPairs(t, ch.ID, true)
	if err := DB.Exec(`DROP TRIGGER reject_resource_snapshot`).Error; err != nil {
		t.Fatal(err)
	}
	if err := DB.Model(&ChannelConfigurationState{}).Where("id = 1").UpdateColumn("revision", maxResourceRevision).Error; err != nil {
		t.Fatal(err)
	}
	placement := catalog.Placements["resource-1"]
	placement.Enabled = false
	if _, err := SaveResourcePlacement(context.Background(), catalog.Revision, "resource-1", placement); !errors.Is(err, ErrChannelRevisionUnavailable) {
		t.Fatalf("exhausted guard allowed partial revocation: %v", err)
	}
	after, err = ReadResourceCatalog(context.Background())
	if err != nil || !reflect.DeepEqual(catalog, after) {
		t.Fatal("revision failure changed inventory")
	}
	assertGatewayPairs(t, ch.ID, true)
}
