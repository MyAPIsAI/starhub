package model

import (
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestRechargeOrderNumbersRemainUniqueAtOneFrozenInstant(t *testing.T) {
	instant := time.Unix(1790000000, 123456789)
	const workers = 16
	numbers := make(chan string, workers*256)
	var group sync.WaitGroup
	for i := 0; i < workers; i++ {
		group.Add(1)
		go func() {
			defer group.Done()
			for j := 0; j < 256; j++ {
				numbers <- generateRechargeOrderNumberAt(instant)
			}
		}()
	}
	group.Wait()
	close(numbers)
	seen := map[string]bool{}
	for number := range numbers {
		if !strings.HasPrefix(number, fmt.Sprintf("ord_%d_", instant.Unix())) || len(number) > 64 || seen[number] {
			t.Fatalf("invalid or duplicate order identifier: %s", number)
		}
		seen[number] = true
	}
	if len(seen) != workers*256 {
		t.Fatal("generated identifiers lost")
	}
}

func TestConcurrentRechargeOrderCreationPersistsDistinctPendingOrders(t *testing.T) {
	newTestDB(t)
	const workers = 8
	const perWorker = 16
	var group sync.WaitGroup
	failures := make(chan error, workers*perWorker)
	for i := 0; i < workers; i++ {
		group.Add(1)
		go func() {
			defer group.Done()
			for j := 0; j < perWorker; j++ {
				failures <- CreateRechargeOrder(&RechargeOrder{UserID: 1, Amount: 1000000000})
			}
		}()
	}
	group.Wait()
	close(failures)
	for err := range failures {
		if err != nil {
			t.Fatal(err)
		}
	}
	var orders []RechargeOrder
	if err := DB.Find(&orders).Error; err != nil || len(orders) != workers*perWorker {
		t.Fatalf("orders lost: count=%d error=%v", len(orders), err)
	}
	seen := map[string]bool{}
	for _, order := range orders {
		if seen[order.OrderNumber] || order.Status != RechargePending || order.ExpiresAt == 0 {
			t.Fatalf("invalid persisted order: %+v", order)
		}
		seen[order.OrderNumber] = true
	}
	for _, table := range []any{&Wallet{}, &LedgerEntry{}} {
		var count int64
		if err := DB.Model(table).Count(&count).Error; err != nil || count != 0 {
			t.Fatalf("creating pending orders credited funds: %d %v", count, err)
		}
	}
}
