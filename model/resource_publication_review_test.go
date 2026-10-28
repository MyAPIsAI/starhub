package model

import (
	"context"
	"encoding/hex"
	"strconv"
	"testing"

	"gorm.io/gorm"

	"starhub/common"
)

func publicationPlan(t *testing.T, bindings []ResourceChannelBinding) *ResourcePublicationPreview {
	t.Helper()
	ctx, err := ReadResourcePublicationContext(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	plan, err := PreviewResourcePublication(context.Background(), publicationExpected(ctx), bindings)
	if err != nil {
		t.Fatal(err)
	}
	return plan
}

func TestResourcePublicationCancelledReadDoesNotReturnAPlan(t *testing.T) {
	_, expected, bindings := publicationFixture(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	const callback = "cancel_resource_review_after_query"
	if err := DB.Callback().Query().After("gorm:after_query").Register(callback, func(tx *gorm.DB) {
		if tx.Statement.Table == "abilities" {
			cancel()
		}
	}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { DB.Callback().Query().Remove(callback) })
	plan, err := PreviewResourcePublication(ctx, expected, bindings)
	if err == nil || plan != nil {
		t.Fatalf("cancelled read returned a usable plan: plan=%t error=%v", plan != nil, err)
	}
}

func TestResourcePublicationReviewBlocksImplicitBindingTransfersAndEmptyBindings(t *testing.T) {
	for _, kind := range []string{"target-owner", "source-owner", "empty-binding"} {
		t.Run(kind, func(t *testing.T) {
			ch, _, bindings := publicationFixture(t)
			code := ""
			switch kind {
			case "target-owner":
				if err := DB.Create(&ResourceGatewayManagedChannel{ChannelID: ch.ID, SourceChannelID: "other-source", Profile: ResourceGatewayTextProfile, Active: false}).Error; err != nil {
					t.Fatal(err)
				}
				code = "gateway_bound_to_other_source"
			case "source-owner":
				other := addChannel(t, "other", "default", "alpha", 0, 1, common.ChannelStatusEnabled)
				if err := DB.Create(&ResourceGatewayManagedChannel{ChannelID: other.ID, SourceChannelID: bindings[0].SourceChannelID, Profile: ResourceGatewayTextProfile, Active: false}).Error; err != nil {
					t.Fatal(err)
				}
				code = "source_bound_to_other_gateway"
			case "empty-binding":
				other := addChannel(t, "other", "default", "alpha", 0, 1, common.ChannelStatusEnabled)
				catalog, err := ReadResourceCatalog(context.Background())
				if err != nil {
					t.Fatal(err)
				}
				snapshot := *catalog.Snapshot
				snapshot.Sequence++
				snapshot.Channels = append(snapshot.Channels, ResourceChannel{ID: "empty-source", Name: "Empty", Enabled: true})
				if _, err := SaveResourceSnapshot(context.Background(), catalog.Revision, snapshot); err != nil {
					t.Fatal(err)
				}
				bindings = append(bindings, ResourceChannelBinding{SourceChannelID: "empty-source", GatewayChannelID: strconv.FormatInt(other.ID, 10)})
				code = "binding_without_supported_pairs"
			}
			plan := publicationPlan(t, bindings)
			if plan.Ready || !publicationIssue(plan, code) {
				t.Fatalf("implicit binding accepted: %+v", plan)
			}
			if kind == "target-owner" {
				if err := DB.Model(&ResourceGatewayManagedChannel{}).Where("channel_id = ?", ch.ID).Update("source_channel_id", "").Error; err != nil {
					t.Fatal(err)
				}
				next := publicationPlan(t, bindings)
				if !next.Ready || next.ReviewDigest == plan.ReviewDigest {
					t.Fatal("explicitly released tombstone cannot be reviewed for a new binding")
				}
				if _, err := GetSatisfiedChannel("default", "alpha"); err != ErrNoChannel {
					t.Fatal("a ready review activated a released tombstone")
				}
			}
		})
	}
}

func TestResourcePublicationReviewIncludesFrozenRouteConstraintsAndGroupState(t *testing.T) {
	ch, _, bindings := publicationFixture(t)
	first := publicationPlan(t, bindings)
	if len(first.ProposedRoutes) != 2 || first.ProposedRoutes[0].ContextLength != 128000 || len(first.ProposedRoutes[0].Capabilities) != 1 || first.ProposedRoutes[0].Capabilities[0] != "text" || first.ProposedRoutes[0].GroupDefinitionID != "0" {
		t.Fatalf("route constraints lost: %+v", first.ProposedRoutes)
	}
	if raw, err := hex.DecodeString(first.ReviewDigest); err != nil || len(raw) != 32 {
		t.Fatalf("invalid review digest: %q", first.ReviewDigest)
	}
	group := &ServiceGroupConfig{GroupID: "default", Name: "First", Enabled: true, Visibility: "common"}
	if err := UpsertServiceGroup(group); err != nil {
		t.Fatal(err)
	}
	second := publicationPlan(t, bindings)
	if !second.Ready || second.ProposedRoutes[0].GroupDefinitionID != strconv.FormatInt(group.ID, 10) || second.ReviewDigest == first.ReviewDigest {
		t.Fatalf("new group identity not sealed: %+v", second)
	}
	if second.ResourceRevision != first.ResourceRevision || second.ChannelRevision != first.ChannelRevision || second.OptionVersion != first.OptionVersion {
		t.Fatal("test unexpectedly changed a version fence")
	}
	group.Name = "Renamed"
	if err := UpsertServiceGroup(group); err != nil {
		t.Fatal(err)
	}
	third := publicationPlan(t, bindings)
	if third.ReviewDigest == second.ReviewDigest {
		t.Fatal("independent metadata revision was absent from review digest")
	}
	if err := DB.Create(&ResourceGatewayManagedChannel{ChannelID: ch.ID, SourceChannelID: bindings[0].SourceChannelID, Profile: ResourceGatewayTextProfile, Active: false}).Error; err != nil {
		t.Fatal(err)
	}
	fourth := publicationPlan(t, bindings)
	if fourth.ReviewDigest == third.ReviewDigest {
		t.Fatal("durable ownership state absent from review digest")
	}
	first.ProposedRoutes[0].Capabilities[0] = "mutated-copy"
	if next := publicationPlan(t, bindings); next.ReviewDigest != fourth.ReviewDigest || next.ProposedRoutes[0].Capabilities[0] != "text" {
		t.Fatal("caller mutated persisted review state")
	}
}

func TestResourcePublicationDuplicatePairsIntersectCapabilitiesAndKeepTightestKnownContext(t *testing.T) {
	_, _, bindings := publicationFixture(t)
	ctx := context.Background()
	catalog, err := ReadResourceCatalog(ctx)
	if err != nil {
		t.Fatal(err)
	}
	snapshot := *catalog.Snapshot
	snapshot.Sequence++
	snapshot.Instances[0].Capabilities = []string{"text", "code"}
	duplicate := snapshot.Instances[0]
	duplicate.ID = "extra-alpha"
	duplicate.Capabilities = []string{"text"}
	duplicate.ContextLength = 32000
	snapshot.Instances = append(snapshot.Instances, duplicate)
	catalog, err = SaveResourceSnapshot(ctx, catalog.Revision, snapshot)
	if err != nil {
		t.Fatal(err)
	}
	name := "alpha"
	catalog, err = SaveResourcePlacement(ctx, catalog.Revision, duplicate.ID, ResourcePlacement{ModelID: &name, GroupIDs: []string{"default"}, Enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	plan := publicationPlan(t, bindings)
	if !plan.Ready || len(plan.ProposedPairs) != 2 || len(plan.ProposedRoutes) != 2 || plan.ProposedRoutes[0].ContextLength != 32000 || len(plan.ProposedRoutes[0].Capabilities) != 1 || plan.ProposedRoutes[0].Capabilities[0] != "text" {
		t.Fatalf("duplicate resources broadened constraints: %+v", plan)
	}
	snapshot.Sequence++
	snapshot.Instances[2].ContextLength = 0
	catalog, err = SaveResourceSnapshot(ctx, catalog.Revision, snapshot)
	if err != nil {
		t.Fatal(err)
	}
	plan = publicationPlan(t, bindings)
	if !plan.Ready || plan.ProposedRoutes[0].ContextLength != 128000 {
		t.Fatalf("unknown source limit erased a known limit: %+v", plan.ProposedRoutes)
	}
	snapshot.Sequence++
	snapshot.Instances[0].Capabilities = []string{"text"}
	snapshot.Instances[2].Capabilities = []string{"code"}
	if _, err = SaveResourceSnapshot(ctx, catalog.Revision, snapshot); err != nil {
		t.Fatal(err)
	}
	plan = publicationPlan(t, bindings)
	if plan.Ready || !publicationIssue(plan, "incompatible_resource_constraints") || len(plan.ProposedPairs) != 1 || len(plan.ProposedRoutes) != 1 || plan.ProposedRoutes[0].Model != "beta" {
		t.Fatalf("incompatible duplicate capability union was accepted: %+v", plan)
	}
}
