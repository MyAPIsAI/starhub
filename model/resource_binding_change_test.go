package model

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"testing"

	"starhub/common"
)

func bindingChangeRequest(t *testing.T, id string, operation string) ResourceBindingChangeRequest {
	t.Helper()
	state, err := ReadResourceBindingState(context.Background(), 1, 100)
	if err != nil {
		t.Fatal(err)
	}
	for _, row := range state.Bindings {
		if row.GatewayChannelID == id {
			return ResourceBindingChangeRequest{ResourceRevision: state.ResourceRevision, ChannelRevision: state.ChannelRevision, SourceChannelID: row.SourceChannelID, PublicationID: row.PublicationID, Operation: operation, RequestID: strings.Repeat("a", 32)}
		}
	}
	t.Fatal("binding missing")
	return ResourceBindingChangeRequest{}
}

func TestResourceBindingChangeRevokeAndReleaseAreAtomicAndHistorical(t *testing.T) {
	for _, operation := range []string{"revoke", "release"} {
		t.Run(operation, func(t *testing.T) {
			ch, _, bindings := publicationFixture(t)
			plan, ack := publishReviewedFixture(t, bindings)
			oldReceipt, err := ReadResourcePublicationReceipt(context.Background(), plan.ReviewDigest)
			if err != nil {
				t.Fatal(err)
			}
			before := publicationDatabaseSnapshot(t)
			request := bindingChangeRequest(t, bindings[0].GatewayChannelID, operation)
			receipt, err := ChangeResourceBinding(context.Background(), bindings[0].GatewayChannelID, request, 9)
			if err != nil {
				t.Fatal(err)
			}
			retained := bindings[0].SourceChannelID
			if operation == "release" {
				retained = ""
			}
			if receipt.Operation != operation || receipt.ActorID != "9" || receipt.SourceChannelID != bindings[0].SourceChannelID || receipt.RetainedSourceChannelID != retained || receipt.PublicationID != ack.PublicationID || receipt.RemovedRouteCount != 2 || receipt.RemovedAbilityCount != 2 || receipt.ResourceRevision != before.Catalog.Revision+1 || receipt.ChannelRevision != before.ChannelRevision+1 || !receipt.WasConfiguredActive || !receipt.Historical || !receipt.Committed || !receipt.ProtectiveMarkerRetained {
				t.Fatalf("incorrect receipt: %+v", receipt)
			}
			after := publicationDatabaseSnapshot(t)
			if len(after.BindingChanges) != 1 || len(after.Owners) != 1 || after.Owners[0].Active || after.Owners[0].SourceChannelID != retained || after.Owners[0].PublicationID != before.Owners[0].PublicationID || len(after.Routes) != 0 || len(after.Abilities) != 0 || !reflect.DeepEqual(before.Channels, after.Channels) || !reflect.DeepEqual(before.Catalog.Snapshot, after.Catalog.Snapshot) || !reflect.DeepEqual(before.Catalog.Placements, after.Catalog.Placements) {
				t.Fatalf("partial or excessive mutation: %+v", after)
			}
			if again, err := ReadResourceBindingChange(context.Background(), request.RequestID); err != nil || !reflect.DeepEqual(receipt, again) {
				t.Fatalf("durable receipt: %+v %v", again, err)
			}
			if operation == "revoke" {
				reused := bindingChangeRequest(t, bindings[0].GatewayChannelID, "release")
				if result, err := ChangeResourceBinding(context.Background(), bindings[0].GatewayChannelID, reused, 10); !errors.Is(err, ErrResourceConflict) || result != nil {
					t.Fatal("request identifier reused with fresh versions or another actor")
				}
				if !reflect.DeepEqual(after, publicationDatabaseSnapshot(t)) {
					t.Fatal("reused request mutated state")
				}
			}
			if again, err := ReadResourcePublicationReceipt(context.Background(), plan.ReviewDigest); err != nil || !reflect.DeepEqual(oldReceipt, again) {
				t.Fatal("binding change rewrote historical publication")
			}
			encoded, _ := json.Marshal(receipt)
			for _, secret := range []string{ch.Key, ch.BaseURL, `"key"`, `"base_url"`} {
				if strings.Contains(string(encoded), secret) {
					t.Fatal("binding receipt exposed credential")
				}
			}
			if err := RebuildAbilities(ch); err != nil {
				t.Fatal(err)
			}
			if err := UpdateChannelStatus(ch.ID, common.ChannelStatusEnabled); err != nil {
				t.Fatal(err)
			}
			assertGatewayPairs(t, ch.ID, false)
			if err := DeleteChannel(ch.ID); err != nil {
				t.Fatal(err)
			}
			if err := InsertChannel(ch); err != nil {
				t.Fatal(err)
			}
			assertGatewayPairs(t, ch.ID, false)
			if _, err := ChangeResourceBinding(context.Background(), bindings[0].GatewayChannelID, request, 9); !errors.Is(err, ErrResourceConflict) {
				t.Fatalf("replay reapplied: %v", err)
			}
			for _, table := range []any{&Wallet{}, &LedgerEntry{}, &Token{}, &Log{}} {
				var count int64
				if err := DB.Model(table).Count(&count).Error; err != nil || count != 0 {
					t.Fatalf("binding change wrote funds/usage: %d %v", count, err)
				}
			}
		})
	}
}

func TestResourceBindingChangeCanReduceAccessWithoutReadablePricingOrInventory(t *testing.T) {
	_, _, bindings := publicationFixture(t)
	publishReviewedFixture(t, bindings)
	request := bindingChangeRequest(t, bindings[0].GatewayChannelID, "release")
	if err := DB.Model(&ResourceCatalogRecord{}).Where("id = 1").Update("snapshot", "corrupt snapshot").Error; err != nil {
		t.Fatal(err)
	}
	if err := DB.Migrator().DropTable(&Option{}); err != nil {
		t.Fatal(err)
	}
	if err := DB.Migrator().DropTable(&OptionRevision{}); err != nil {
		t.Fatal(err)
	}
	result, err := ChangeResourceBinding(context.Background(), bindings[0].GatewayChannelID, request, 9)
	if err != nil || result.RetainedSourceChannelID != "" || result.RemovedRouteCount != 2 {
		t.Fatalf("unrelated read state blocked access reduction: %+v %v", result, err)
	}
	var catalog ResourceCatalogRecord
	if err := DB.First(&catalog, 1).Error; err != nil || catalog.Snapshot != "corrupt snapshot" || catalog.Revision != request.ResourceRevision+1 {
		t.Fatal("reduction rewrote inventory")
	}
	var owner ResourceGatewayManagedChannel
	if err := DB.First(&owner, "channel_id = ?", bindings[0].GatewayChannelID).Error; err != nil || owner.Active || owner.SourceChannelID != "" {
		t.Fatal("protective ownership lost")
	}
	for _, table := range []any{&Ability{}, &ResourceGatewayRoute{}} {
		var count int64
		if err := DB.Model(table).Count(&count).Error; err != nil || count != 0 {
			t.Fatal("routing remained after reduction")
		}
	}
}

func TestResourceBindingChangeReleaseAllowsOnlyExplicitRepublishToAnotherGateway(t *testing.T) {
	old, _, bindings := publicationFixture(t)
	publishReviewedFixture(t, bindings)
	next := addChannel(t, "Next gateway", "default,vip", "alpha,beta", 0, 1, common.ChannelStatusEnabled)
	transfer := []ResourceChannelBinding{{SourceChannelID: bindings[0].SourceChannelID, GatewayChannelID: strconv.FormatInt(next.ID, 10)}}
	if plan := publicationPlan(t, transfer); plan.Ready || !publicationIssue(plan, "source_bound_to_other_gateway") {
		t.Fatal("implicit transfer allowed")
	}
	request := bindingChangeRequest(t, bindings[0].GatewayChannelID, "revoke")
	if _, err := ChangeResourceBinding(context.Background(), bindings[0].GatewayChannelID, request, 9); err != nil {
		t.Fatal(err)
	}
	if plan := publicationPlan(t, transfer); plan.Ready {
		t.Fatal("revocation silently released ownership")
	}
	request = bindingChangeRequest(t, bindings[0].GatewayChannelID, "release")
	request.RequestID = strings.Repeat("b", 32)
	if _, err := ChangeResourceBinding(context.Background(), bindings[0].GatewayChannelID, request, 9); err != nil {
		t.Fatal(err)
	}
	if plan := publicationPlan(t, transfer); !plan.Ready {
		t.Fatalf("explicit release did not permit review: %+v", plan.Issues)
	}
	publishReviewedFixture(t, transfer)
	if err := RebuildAbilities(old); err != nil {
		t.Fatal(err)
	}
	if got, err := GetSatisfiedChannelPreferred("default", "alpha", nil, old.ID); err != nil || got.ID != next.ID {
		t.Fatalf("released target inherited grants: %+v %v", got, err)
	}
}

func TestResourceBindingChangeRevalidatesGuardsAndRejectsInvalidRequests(t *testing.T) {
	_, _, bindings := publicationFixture(t)
	publishReviewedFixture(t, bindings)
	request := bindingChangeRequest(t, bindings[0].GatewayChannelID, "release")
	before := publicationDatabaseSnapshot(t)
	for _, change := range []string{"resource", "channel", "source", "publication", "request-id", "operation", "actor", "target", "version-limit"} {
		input := request
		id := bindings[0].GatewayChannelID
		actor := int64(9)
		want := ErrResourceConflict
		switch change {
		case "resource":
			input.ResourceRevision--
		case "channel":
			input.ChannelRevision--
		case "source":
			input.SourceChannelID = "other"
		case "publication":
			input.PublicationID = "0"
		case "request-id":
			input.RequestID = "bad"
			want = ErrInvalidResource
		case "operation":
			input.Operation = "delete"
			want = ErrInvalidResource
		case "actor":
			actor = 0
			want = ErrInvalidResource
		case "target":
			id = "01"
			want = ErrInvalidResource
		case "version-limit":
			input.ResourceRevision = maxResourceRevision
			want = ErrInvalidResource
		}
		if receipt, err := ChangeResourceBinding(context.Background(), id, input, actor); !errors.Is(err, want) || receipt != nil {
			t.Fatalf("%s guard: %+v %v", change, receipt, err)
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if receipt, err := ChangeResourceBinding(ctx, bindings[0].GatewayChannelID, request, 9); err == nil || receipt != nil {
		t.Fatal("cancelled write succeeded")
	}
	if !reflect.DeepEqual(before, publicationDatabaseSnapshot(t)) {
		t.Fatal("rejected change left partial state")
	}
}

func TestResourceBindingChangeConcurrentSingleWinnerAndRollbackEveryStage(t *testing.T) {
	t.Run("concurrent", func(t *testing.T) {
		_, _, bindings := publicationFixture(t)
		publishReviewedFixture(t, bindings)
		request := bindingChangeRequest(t, bindings[0].GatewayChannelID, "release")
		var wg sync.WaitGroup
		results := make(chan error, 8)
		for i := 0; i < 8; i++ {
			wg.Add(1)
			go func(i int) {
				defer wg.Done()
				input := request
				input.RequestID = fmt.Sprintf("%032x", i+1)
				_, err := ChangeResourceBinding(context.Background(), bindings[0].GatewayChannelID, input, 9)
				results <- err
			}(i)
		}
		wg.Wait()
		close(results)
		won := 0
		for err := range results {
			if err == nil {
				won++
			} else if !errors.Is(err, ErrResourceConflict) {
				t.Fatal(err)
			}
		}
		if won != 1 || len(publicationDatabaseSnapshot(t).BindingChanges) != 1 {
			t.Fatalf("winners=%d", won)
		}
	})
	for _, stage := range []string{"receipt", "owner", "routes", "abilities", "channel-version", "resource-version"} {
		t.Run(stage, func(t *testing.T) {
			_, _, bindings := publicationFixture(t)
			publishReviewedFixture(t, bindings)
			request := bindingChangeRequest(t, bindings[0].GatewayChannelID, "release")
			before := publicationDatabaseSnapshot(t)
			statements := map[string]string{
				"receipt":          `BEFORE INSERT ON resource_gateway_binding_changes`,
				"owner":            `BEFORE UPDATE ON resource_gateway_managed_channels`,
				"routes":           `BEFORE DELETE ON resource_gateway_routes`,
				"abilities":        `BEFORE DELETE ON abilities`,
				"channel-version":  `BEFORE UPDATE ON channel_configuration_states`,
				"resource-version": `BEFORE UPDATE OF revision ON resource_catalog_records WHEN NEW.revision <> OLD.revision`,
			}
			if err := DB.Exec("CREATE TRIGGER fail_binding_change " + statements[stage] + " BEGIN SELECT RAISE(ABORT,'injected'); END").Error; err != nil {
				t.Fatal(err)
			}
			if receipt, err := ChangeResourceBinding(context.Background(), bindings[0].GatewayChannelID, request, 9); err == nil || receipt != nil {
				t.Fatalf("failed %s returned receipt", stage)
			}
			if !reflect.DeepEqual(before, publicationDatabaseSnapshot(t)) {
				t.Fatalf("failed %s left partial state", stage)
			}
		})
	}
}

func TestResourceBindingChangeRestartOrphanReleaseAndCorruptReceipt(t *testing.T) {
	ch, _, bindings := publicationFixture(t)
	publishReviewedFixture(t, bindings)
	if err := DeleteChannel(ch.ID); err != nil {
		t.Fatal(err)
	}
	request := bindingChangeRequest(t, bindings[0].GatewayChannelID, "release")
	receipt, err := ChangeResourceBinding(context.Background(), bindings[0].GatewayChannelID, request, 9)
	if err != nil || receipt.RemovedRouteCount != 0 || receipt.WasConfiguredActive {
		t.Fatalf("orphan release: %+v %v", receipt, err)
	}
	var databases []struct {
		Name string
		File string
	}
	if err := DB.Raw("PRAGMA database_list").Scan(&databases).Error; err != nil {
		t.Fatal(err)
	}
	path := ""
	for _, database := range databases {
		if database.Name == "main" {
			path = database.File
		}
	}
	if path == "" {
		t.Fatal("database path missing")
	}
	if err := CloseDB(); err != nil {
		t.Fatal(err)
	}
	if err := InitDB(path); err != nil {
		t.Fatal(err)
	}
	if again, err := ReadResourceBindingChange(context.Background(), request.RequestID); err != nil || !reflect.DeepEqual(receipt, again) {
		t.Fatalf("receipt lost on restart: %+v %v", again, err)
	}
	if err := InsertChannel(ch); err != nil {
		t.Fatal(err)
	}
	assertGatewayPairs(t, ch.ID, false)
	if _, err := ReadResourceBindingChange(context.Background(), strings.Repeat("b", 32)); !errors.Is(err, ErrResourceBindingChangeNotFound) {
		t.Fatalf("missing receipt: %v", err)
	}
	if _, err := ReadResourceBindingChange(context.Background(), "invalid"); !errors.Is(err, ErrInvalidResource) {
		t.Fatal("invalid request id accepted")
	}
	if err := DB.Model(&ResourceGatewayBindingChange{}).Where("request_id = ?", request.RequestID).Update("retained_source_channel_id", "corrupt").Error; err != nil {
		t.Fatal(err)
	}
	if receipt, err := ReadResourceBindingChange(context.Background(), request.RequestID); err == nil || receipt != nil {
		t.Fatal("corrupt receipt trusted")
	}
}
