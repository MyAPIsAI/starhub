package model

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"starhub/common"
	"starhub/config"
)

func TestM7ConcurrentAccountLimitIsAtomicAndReleases(t *testing.T) {
	newTestDB(t)
	user := &User{Username: "concurrent-customer", Status: 1, Role: 1, ConcurrencyLimit: 3}
	if err := CreateUser(user); err != nil {
		t.Fatal(err)
	}
	var lock sync.Mutex
	var acquired []func()
	var failures []error
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			release, err := AcquireCustomerRequest(context.Background(), user.ID)
			lock.Lock()
			defer lock.Unlock()
			if err == nil {
				acquired = append(acquired, release)
			} else {
				failures = append(failures, err)
			}
		}()
	}
	wg.Wait()
	defer func() {
		for _, release := range acquired {
			release()
		}
	}()
	if len(acquired) != 3 {
		t.Fatalf("accepted %d requests, expected exactly 3", len(acquired))
	}
	for _, err := range failures {
		if !errors.Is(err, ErrCustomerConcurrency) {
			t.Fatal(err)
		}
	}
	acquired[0]()
	acquired = acquired[1:]
	release, err := AcquireCustomerRequest(context.Background(), user.ID)
	if err != nil {
		t.Fatal(err)
	}
	acquired = append(acquired, release)
}

func TestM7TimezoneChangePreservesActiveQuotaPeriod(t *testing.T) {
	newTestDB(t)
	_ = config.SetRegionTimezone("UTC")
	t.Cleanup(func() { _ = config.SetRegionTimezone("UTC") })
	start := time.Date(2026, 11, 4, 0, 0, 0, 0, time.UTC)
	token := &Token{UserID: 1, Name: "Regional daily limit", Status: 1, QuotaPeriod: "daily", QuotaTimezone: "UTC", QuotaLimit: 1000, RemainQuota: 400, PeriodUsedQuota: 600, PeriodStartedAt: start.Unix()}
	if err := InsertToken(token); err != nil {
		t.Fatal(err)
	}
	p := DefaultDeploymentProfile()
	p.Timezone = "Asia/Shanghai"
	if _, err := SaveDeploymentProfile(context.Background(), p, 0, 0, ""); err != nil {
		t.Fatal(err)
	}
	if err := RefreshTokenPeriod(token, start.Add(20*time.Hour)); err != nil {
		t.Fatal(err)
	}
	if token.PeriodUsedQuota != 600 || token.QuotaTimezone != "UTC" {
		t.Fatal("profile change reset an active quota period")
	}
	if err := RefreshTokenPeriod(token, start.Add(24*time.Hour)); err != nil {
		t.Fatal(err)
	}
	if token.PeriodUsedQuota != 0 || token.RemainQuota != 1000 || token.QuotaTimezone != "Asia/Shanghai" {
		t.Fatalf("new cycle failed to adopt timezone: %+v", token)
	}
	var history TokenQuotaPeriod
	if err := DB.First(&history, "token_id = ?", token.ID).Error; err != nil || history.UsedQuota != 600 || history.EndedAt != start.Add(24*time.Hour).Unix() {
		t.Fatal("historical quota period changed")
	}
}

func TestM7FailedModelSaveDoesNotCommitPricingOrHistory(t *testing.T) {
	newTestDB(t)
	if err := SeedOptions(); err != nil {
		t.Fatal(err)
	}
	before, err := ReadPricingOptions(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	_, err = SaveStandardModel(context.Background(), StandardModelWrite{StandardModel: StandardModel{Model: "no-resource", Name: "No resource", Vendor: "OpenAI", Kind: "chat", Protocols: []string{"OpenAI"}, Listed: true}, ExpectedPricingVersion: before.Version, PricingOptions: []Option{{Key: "model_ratio", Value: `{"no-resource":2}`}}}, 0, "")
	if !errors.Is(err, ErrAdminInvalid) {
		t.Fatal("listing without a resource was accepted")
	}
	after, err := ReadPricingOptions(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if before.Version != after.Version {
		t.Fatal("failed listing committed a pricing change")
	}
	var count int64
	DB.Model(&StandardModelHistory{}).Count(&count)
	if count != 0 {
		t.Fatal("failed listing created history")
	}
}

func TestM7TaskRoutingEvidenceRemainsFrozen(t *testing.T) {
	newTestDB(t)
	ctx := context.WithValue(context.Background(), common.CtxAdmissionRouting, &RoutingEvidence{Sequence: 30, Revision: 9, Resources: map[string][]string{"3|model|group": {"instance-1"}}})
	task := &Task{ChannelID: 3, Model: "model", Group: "group"}
	FreezeTaskRouting(ctx, task, true)
	if err := InsertTask(task); err != nil {
		t.Fatal(err)
	}
	var err error
	task, err = GetTaskByID(task.ID)
	if err != nil {
		t.Fatal(err)
	}
	entry := &Log{}
	ApplyTaskRouting(task, entry)
	if entry.ResourceID != "instance-1" || entry.SnapshotSequence != 30 || entry.ResourceRevision != 9 || !entry.AffinityUsed {
		t.Fatal("durable task lost accepted routing identity")
	}
}
