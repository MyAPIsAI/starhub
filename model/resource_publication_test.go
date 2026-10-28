package model

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strconv"
	"strings"
	"testing"

	"starhub/common"
	"starhub/setting"
)

func publicationFixture(t *testing.T) (*Channel, ResourcePublicationPrecondition, []ResourceChannelBinding) {
	t.Helper()
	newTestDB(t)
	if err := SeedOptions(); err != nil {
		t.Fatal(err)
	}
	if _, err := SaveOption(setting.OptionKeyGroupRatio, `{"default":1,"vip":0.8}`, 0); err != nil {
		t.Fatal(err)
	}
	ch := addChannel(t, "Gateway", "default,vip", "alpha,beta", 0, 1, common.ChannelStatusEnabled)
	ch.Key, ch.BaseURL = "private-upstream-credential", "https://private.invalid/secret-path"
	if err := UpdateChannel(ch); err != nil {
		t.Fatal(err)
	}
	snapshot := resourceFixture()
	snapshot.Channels[0].ID = strconv.FormatInt(ch.ID, 10)
	snapshot.Instances[0].ChannelID = snapshot.Channels[0].ID
	snapshot.Instances[0].CanonicalModel, snapshot.Instances[0].UpstreamModel = "alpha", "alpha"
	second := snapshot.Instances[0]
	second.ID, second.CanonicalModel, second.UpstreamModel = "resource-2", "beta", "beta"
	snapshot.Instances = append(snapshot.Instances, second)
	if _, err := SaveResourceSnapshot(context.Background(), 0, snapshot); err != nil {
		t.Fatal(err)
	}
	for i, resource := range snapshot.Instances {
		group := []string{"default"}
		if i == 1 {
			group = []string{"vip"}
		}
		if _, err := SaveResourcePlacement(context.Background(), int64(i+1), resource.ID, ResourcePlacement{ModelID: &resource.CanonicalModel, GroupIDs: group, Enabled: true}); err != nil {
			t.Fatal(err)
		}
	}
	context, err := ReadResourcePublicationContext(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	return ch, publicationExpected(context), []ResourceChannelBinding{{SourceChannelID: snapshot.Channels[0].ID, GatewayChannelID: strconv.FormatInt(ch.ID, 10)}}
}

func publicationExpected(state *ResourcePublicationContext) ResourcePublicationPrecondition {
	return ResourcePublicationPrecondition{ResourceRevision: state.ResourceRevision, ChannelRevision: state.ChannelRevision, OptionVersion: state.OptionVersion}
}

func publicationIssue(preview *ResourcePublicationPreview, code string) bool {
	for _, issue := range preview.Issues {
		if issue.Code == code {
			return true
		}
	}
	return false
}

func TestResourcePublicationPreviewExactPairsAndNoMutation(t *testing.T) {
	ch, expected, bindings := publicationFixture(t)
	var abilities []Ability
	if err := DB.Order("model, `group`").Find(&abilities).Error; err != nil {
		t.Fatal(err)
	}
	before, err := ReadResourceCatalog(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	preview, err := PreviewResourcePublication(context.Background(), expected, bindings)
	if err != nil || !preview.Ready || !preview.PreviewOnly || preview.AffectsGatewayRouting || preview.Schema != ResourcePublicationPreviewSchema {
		t.Fatalf("preview: %+v %v", preview, err)
	}
	want := []ResourcePublicationPair{{GatewayChannelID: strconv.FormatInt(ch.ID, 10), Model: "alpha", Group: "default"}, {GatewayChannelID: strconv.FormatInt(ch.ID, 10), Model: "beta", Group: "vip"}}
	if !reflect.DeepEqual(preview.ProposedPairs, want) || len(preview.ExistingPairs) != 4 {
		t.Fatalf("cross-product introduced or current routes lost: %+v", preview)
	}
	for _, pair := range preview.ExistingPairs {
		if !pair.Enabled {
			t.Fatal("enabled ability reported disabled")
		}
	}
	for i := 0; i < 5; i++ {
		next, err := PreviewResourcePublication(context.Background(), expected, bindings)
		if err != nil || !reflect.DeepEqual(next, preview) {
			t.Fatalf("unstable preview: %+v %v", next, err)
		}
	}
	encoded, _ := json.Marshal(preview)
	for _, secret := range []string{ch.Key, ch.BaseURL, "private.invalid", `"key"`, `"base_url"`} {
		if strings.Contains(string(encoded), secret) {
			t.Fatalf("publication context leaked %q", secret)
		}
	}
	after, _ := ReadResourceCatalog(context.Background())
	stored, _ := GetChannelByID(ch.ID)
	var afterAbilities []Ability
	if err := DB.Order("model, `group`").Find(&afterAbilities).Error; err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(before, after) || !reflect.DeepEqual(ch, stored) || !reflect.DeepEqual(abilities, afterAbilities) || channelRevision(t) != expected.ChannelRevision {
		t.Fatal("preview changed actual configuration")
	}
	// A numeric source ID still requires a binding, even when it equals the gateway ID.
	unbound := []ResourceChannelBinding{{SourceChannelID: "other-source", GatewayChannelID: bindings[0].GatewayChannelID}}
	preview, err = PreviewResourcePublication(context.Background(), expected, unbound)
	if err != nil || preview.Ready || len(preview.ProposedPairs) != 0 || !publicationIssue(preview, "unbound_source_channel") {
		t.Fatalf("source ID was guessed: %+v %v", preview, err)
	}
}

func TestResourcePublicationPreviewRejectsStaleDependencies(t *testing.T) {
	for _, change := range []string{"resource", "channel", "channel-aba", "delete-recreate", "abilities", "key", "option"} {
		t.Run(change, func(t *testing.T) {
			ch, expected, bindings := publicationFixture(t)
			var err error
			switch change {
			case "resource":
				_, err = SaveResourcePlacement(context.Background(), expected.ResourceRevision, "resource-1", ResourcePlacement{GroupIDs: []string{}})
			case "channel":
				err = UpdateChannelStatus(ch.ID, common.ChannelStatusManuallyDisabled)
			case "channel-aba":
				ch.Name = "changed"
				err = UpdateChannel(ch)
				if err == nil {
					ch.Name = "Gateway"
					err = UpdateChannel(ch)
				}
			case "delete-recreate":
				err = DeleteChannel(ch.ID)
				if err == nil {
					err = InsertChannel(ch)
				}
			case "abilities":
				err = SetAbilitiesEnabledByChannel(ch.ID, false)
			case "key":
				_, err = DisableChannelKey(ch.ID, 0)
			case "option":
				_, err = SaveOption(setting.OptionKeyCacheRatio, "0.75", 0)
			}
			if err != nil {
				t.Fatal(err)
			}
			if _, err := PreviewResourcePublication(context.Background(), expected, bindings); !errors.Is(err, ErrResourceConflict) {
				t.Fatalf("stale %s accepted: %v", change, err)
			}
		})
	}
}

func TestResourcePublicationPreviewFailsClosedForUnsupportedInventory(t *testing.T) {
	for _, change := range []string{"alias", "placement-model", "protocol", "capability", "disabled-source", "disabled-gateway", "unavailable", "unpriced-group", "disabled-group", "deleted-group", "unknown-visibility", "duplicate-resource"} {
		t.Run(change, func(t *testing.T) {
			ch, _, bindings := publicationFixture(t)
			catalog, _ := ReadResourceCatalog(context.Background())
			snapshot := *catalog.Snapshot
			snapshot.Sequence++
			expectedCode := ""
			switch change {
			case "alias":
				snapshot.Instances[0].UpstreamModel = "other-upstream"
				expectedCode = "model_alias_not_supported"
			case "placement-model":
				expectedCode = "model_alias_not_supported"
			case "protocol":
				snapshot.Instances[0].Protocols = []string{"Anthropic"}
				expectedCode = "protocol_or_capability_not_supported"
			case "capability":
				snapshot.Instances[0].Capabilities = []string{"text", "vision"}
				expectedCode = "protocol_or_capability_not_supported"
			case "disabled-source":
				snapshot.Channels[0].Enabled = false
				expectedCode = "unavailable_resource_or_channel"
			case "disabled-gateway":
				if err := UpdateChannelStatus(ch.ID, 2); err != nil {
					t.Fatal(err)
				}
				expectedCode = "unavailable_resource_or_channel"
			case "unavailable":
				snapshot.Instances[0].Status = "unavailable"
				expectedCode = "unavailable_resource_or_channel"
			case "unpriced-group":
				if _, err := SaveOption(setting.OptionKeyGroupRatio, `{"vip":1}`, 0); err != nil {
					t.Fatal(err)
				}
				expectedCode = "group_unavailable_or_unpriced"
			case "disabled-group", "unknown-visibility":
				definition := &ServiceGroupConfig{GroupID: "default", Enabled: change != "disabled-group", Visibility: "unknown"}
				if err := UpsertServiceGroup(definition); err != nil {
					t.Fatal(err)
				}
				expectedCode = "group_unavailable_or_unpriced"
			case "deleted-group":
				if err := DB.Create(&ServiceGroupDeletion{GroupID: "default"}).Error; err != nil {
					t.Fatal(err)
				}
				expectedCode = "group_unavailable_or_unpriced"
			case "duplicate-resource":
				duplicate := snapshot.Instances[0]
				duplicate.ID = "duplicate"
				snapshot.Instances = append(snapshot.Instances, duplicate)
			}
			state, err := SaveResourceSnapshot(context.Background(), catalog.Revision, snapshot)
			if err != nil {
				t.Fatal(err)
			}
			// Explicitly restore an alias placement after import disabled it, proving that preview still rejects it.
			if change == "alias" || change == "placement-model" || change == "duplicate-resource" {
				modelID := "alpha"
				resourceID := "resource-1"
				if change == "placement-model" {
					modelID = "different-model"
				}
				if change == "duplicate-resource" {
					resourceID = "duplicate"
				}
				state, err = SaveResourcePlacement(context.Background(), state.Revision, resourceID, ResourcePlacement{ModelID: &modelID, GroupIDs: []string{"default"}, Enabled: true})
				if err != nil {
					t.Fatal(err)
				}
			}
			current, err := ReadResourcePublicationContext(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			preview, err := PreviewResourcePublication(context.Background(), publicationExpected(current), bindings)
			if err != nil {
				t.Fatal(err)
			}
			if change == "duplicate-resource" {
				if !preview.Ready || len(preview.ProposedPairs) != 2 {
					t.Fatalf("duplicates not collapsed: %+v", preview)
				}
				return
			}
			if preview.Ready || !publicationIssue(preview, expectedCode) {
				t.Fatalf("unsafe %s accepted: %+v", change, preview)
			}
		})
	}
}

func TestResourcePublicationPreviewValidatesBindingsAndMissingState(t *testing.T) {
	_, expected, bindings := publicationFixture(t)
	for _, invalid := range [][]ResourceChannelBinding{nil, {}, {{SourceChannelID: "1", GatewayChannelID: "01"}}, {{SourceChannelID: "1", GatewayChannelID: "0"}}, {{SourceChannelID: "1", GatewayChannelID: "9223372036854775808"}}, {bindings[0], bindings[0]}, {bindings[0], {SourceChannelID: "other", GatewayChannelID: bindings[0].GatewayChannelID}}} {
		if _, err := PreviewResourcePublication(context.Background(), expected, invalid); !errors.Is(err, ErrInvalidResource) {
			t.Fatalf("invalid binding accepted: %+v %v", invalid, err)
		}
	}
	for _, invalid := range []ResourcePublicationPrecondition{{ResourceRevision: -1}, {ChannelRevision: maxResourceRevision + 1}, {OptionVersion: -1}} {
		if _, err := PreviewResourcePublication(context.Background(), invalid, bindings); !errors.Is(err, ErrInvalidResource) {
			t.Fatalf("invalid revision accepted: %v", err)
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := PreviewResourcePublication(ctx, expected, bindings); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancel ignored: %v", err)
	}
	if err := DB.Where("`key` = ?", setting.OptionKeyGroupRatio).Delete(&Option{}).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := ReadResourcePublicationContext(context.Background()); err == nil {
		t.Fatal("missing authoritative ratios invented a default")
	}
}
