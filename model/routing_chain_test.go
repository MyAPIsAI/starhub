package model

import (
	"errors"
	"fmt"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func TestRoutingChainExpiryAndRenewal(t *testing.T) {
	newTestDB(t)
	const now int64 = 2000000000
	if _, err := admitRoutingChain(1, "default", "chain", "first", false, now); err != nil {
		t.Fatal(err)
	}
	if _, err := admitRoutingChain(1, "default", "chain", "second", true, now+10); err != nil {
		t.Fatal(err)
	}
	var chain RoutingChain
	if err := DB.First(&chain).Error; err != nil {
		t.Fatal(err)
	}
	if chain.Model != "first" || chain.ExpiresAt != now+86400 {
		t.Fatalf("conflict changed pin or extended expiry: %+v", chain)
	}
	if winner, err := admitRoutingChain(1, "default", "chain", "first", true, now+100); err != nil || winner != "first" {
		t.Fatalf("renewal=%q %v", winner, err)
	}
	if err := DB.First(&chain).Error; err != nil {
		t.Fatal(err)
	}
	if chain.ExpiresAt != now+86500 {
		t.Fatalf("idle period not renewed: %+v", chain)
	}
	if _, err := admitRoutingChain(1, "default", "chain", "first", true, chain.ExpiresAt); !errors.Is(err, ErrRoutingChainExpired) {
		t.Fatalf("expiry boundary accepted: %v", err)
	}
	if err := CleanupExpiredRoutingChains(chain.ExpiresAt); err != nil {
		t.Fatal(err)
	}
	if _, err := admitRoutingChain(1, "default", "chain", "second", true, chain.ExpiresAt); !errors.Is(err, ErrRoutingChainExpired) {
		t.Fatalf("purged continuation was rebound: %v", err)
	}
	if _, err := admitRoutingChain(1, "other", "chain", "second", true, now); !errors.Is(err, ErrRoutingChainExpired) {
		t.Fatalf("group isolation lost: %v", err)
	}
	if _, err := admitRoutingChain(2, "default", "chain", "second", true, now); !errors.Is(err, ErrRoutingChainExpired) {
		t.Fatalf("token isolation lost: %v", err)
	}
}

func TestRoutingChainConcurrentCapacity(t *testing.T) {
	newTestDB(t)
	now := time.Now().Unix()
	chains := make([]RoutingChain, MaxRoutingChainsPerToken-1)
	for i := range chains {
		chains[i] = RoutingChain{TokenID: 1, Group: "default", ChainHash: routingChainHash(fmt.Sprint(i)), Model: "first", ExpiresAt: now + 3600}
	}
	if err := DB.CreateInBatches(chains, 100).Error; err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	results := make(chan error, 12)
	for i := 0; i < 12; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, err := admitRoutingChain(1, fmt.Sprintf("group-%d", i), "new", "first", false, now)
			results <- err
		}(i)
	}
	wg.Wait()
	close(results)
	winners := 0
	for err := range results {
		if err == nil {
			winners++
		} else if !errors.Is(err, ErrRoutingChainLimit) {
			t.Fatal(err)
		}
	}
	if winners != 1 {
		t.Fatalf("capacity winners=%d want=1", winners)
	}
	if _, err := admitRoutingChain(1, "default", "0", "first", true, now+1); err != nil {
		t.Fatalf("existing chain blocked at capacity: %v", err)
	}
	if _, err := admitRoutingChain(2, "default", "new", "first", false, now); err != nil {
		t.Fatalf("capacity leaked across tokens: %v", err)
	}
	if err := DB.Model(&RoutingChain{}).Where("token_id = ? AND chain_hash = ?", 1, routingChainHash("1")).UpdateColumn("expires_at", now).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := admitRoutingChain(1, "default", "after-cleanup", "first", false, now); err != nil {
		t.Fatalf("expired capacity not reclaimed: %v", err)
	}
}

func TestRoutingChainConcurrentWinner(t *testing.T) {
	newTestDB(t)
	var wg sync.WaitGroup
	results := make(chan string, 12)
	for i := 0; i < 12; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			winner, err := PinRoutingChain(1, "default", "same", fmt.Sprint(i))
			if err != nil {
				t.Error(err)
			}
			results <- winner
		}(i)
	}
	wg.Wait()
	close(results)
	first := ""
	for winner := range results {
		if first == "" {
			first = winner
		}
		if winner == "" || winner != first {
			t.Fatalf("inconsistent winners: %q and %q", first, winner)
		}
	}
	var count int64
	if err := DB.Model(&RoutingChain{}).Count(&count).Error; err != nil || count != 1 {
		t.Fatalf("count=%d err=%v", count, err)
	}
}

func TestRoutingChainMigrationAndTokenDeletion(t *testing.T) {
	path := filepath.Join(t.TempDir(), "migration.db")
	if err := InitDB(path); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = CloseDB() })
	legacy := RoutingChain{TokenID: 1, Group: "default", ChainHash: routingChainHash("legacy"), Model: "first"}
	if err := DB.Create(&legacy).Error; err != nil {
		t.Fatal(err)
	}
	if err := CloseDB(); err != nil {
		t.Fatal(err)
	}
	before := time.Now().Unix()
	if err := InitDB(path); err != nil {
		t.Fatal(err)
	}
	if err := DB.First(&legacy).Error; err != nil {
		t.Fatal(err)
	}
	if legacy.ExpiresAt < before+86400 {
		t.Fatalf("legacy pin not migrated: %+v", legacy)
	}
	if winner, err := FindRoutingChain(1, "default", "legacy"); err != nil || winner != "first" {
		t.Fatalf("legacy continuity lost: %q %v", winner, err)
	}
	if _, err := PinRoutingChain(2, "default", "other-token", "second"); err != nil {
		t.Fatal(err)
	}
	if err := DeleteToken(1); err != nil {
		t.Fatal(err)
	}
	if winner, err := FindRoutingChain(1, "default", "legacy"); err != nil || winner != "" {
		t.Fatalf("deleted token retained pin: %q %v", winner, err)
	}
	if winner, err := FindRoutingChain(2, "default", "other-token"); err != nil || winner != "second" {
		t.Fatalf("other token pin removed: %q %v", winner, err)
	}
}
