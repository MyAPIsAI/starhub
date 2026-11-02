package model

import (
	"errors"
	"fmt"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"

	"starhub/common"
)

func TestTokenCreationLimitSerializesLegacyAndOwnedPaths(t *testing.T) {
	newTestDB(t)
	owner := &User{Username: "limited-owner", Status: 1, Group: "default", AffCode: "limited-owner"}
	if err := CreateUser(owner); err != nil {
		t.Fatal(err)
	}
	if err := InsertChannel(&Channel{Name: "choices", Type: 1, Status: 1, Key: "synthetic", Models: "alpha", Group: "default"}); err != nil {
		t.Fatal(err)
	}
	choices, err := ReadOwnedTokenChoices(owner.ID)
	if err != nil {
		t.Fatal(err)
	}
	rows := make([]Token, MaxTokensPerUser-1)
	for i := range rows {
		rows[i] = Token{UserID: owner.ID, Key: fmt.Sprintf("synthetic-limit-%d", i), Status: common.StatusDisabled, ExpiredTime: 1}
	}
	if err := DB.CreateInBatches(rows, 100).Error; err != nil {
		t.Fatal(err)
	}
	var winners atomic.Int64
	var wg sync.WaitGroup
	for i := range 12 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			var err error
			if i%2 == 0 {
				err = InsertToken(&Token{UserID: owner.ID, Name: "legacy", Status: 1, ExpiredTime: -1})
			} else {
				_, _, err = MutateOwnedToken(owner.ID, fmt.Sprint("limit-", i), "create", 0, "", choices.Revision, map[string]any{"name": "owned", "group": "default", "model_limits": "alpha"})
			}
			if err == nil {
				winners.Add(1)
			} else if !errors.Is(err, ErrTokenLimitReached) {
				t.Errorf("creation failed for an unexpected reason: %v", err)
			}
		}()
	}
	wg.Wait()
	var count int64
	if err := DB.Model(&Token{}).Where("user_id = ?", owner.ID).Count(&count).Error; err != nil || winners.Load() != 1 || count != MaxTokensPerUser {
		t.Fatalf("limit was not atomic: winners=%d count=%d err=%v", winners.Load(), count, err)
	}
	if err := InsertToken(&Token{UserID: owner.ID + 1, Name: "other owner"}); err != nil {
		t.Fatalf("another owner shared the limit: %v", err)
	}
	if err := DeleteToken(rows[0].ID); err != nil {
		t.Fatal(err)
	}
	if err := InsertToken(&Token{UserID: owner.ID, Name: "replacement"}); err != nil {
		t.Fatalf("deleting a key did not free a slot: %v", err)
	}
}

func TestTokenSmartPolicyPreservesLegacyAndRejectsExpandedCandidates(t *testing.T) {
	for _, candidates := range []string{"gamma", "alpha,gamma", "alpha,alpha", "myapis-auto", "veyra-auto", " , "} {
		if err := ValidateTokenSmartPolicy(&Token{ModelLimits: "alpha,beta", SmartCandidates: candidates}); err == nil {
			t.Fatalf("invalid smart candidates accepted: %q", candidates)
		}
	}
	for _, candidates := range []string{"", "alpha", "alpha,beta"} {
		if err := ValidateTokenSmartPolicy(&Token{ModelLimits: "alpha,beta", SmartCandidates: candidates}); err != nil {
			t.Fatalf("valid smart candidates rejected: %q %v", candidates, err)
		}
	}
	legacy := Token{ModelLimits: "alpha,beta"}
	if !legacy.AllowsSmartRouting() {
		t.Fatal("migration disabled a legacy key")
	}
	disabled := false
	legacy.SmartAccess = &disabled
	if legacy.AllowsSmartRouting() || !legacy.AllowsModel("alpha") {
		t.Fatal("smart permission was not independent from direct model access")
	}
}

func TestLegacyTokenPolicyRevisionPreventsMixedConcurrentUpdates(t *testing.T) {
	newTestDB(t)
	token := &Token{UserID: 1, Name: "policy race", ModelLimits: "alpha,beta"}
	if err := InsertToken(token); err != nil {
		t.Fatal(err)
	}
	var successes atomic.Int64
	var wg sync.WaitGroup
	for _, fields := range []map[string]any{{"model_limits": "alpha"}, {"smart_candidates": "beta"}} {
		wg.Add(1)
		go func() {
			defer wg.Done()
			err := UpdateTokenFieldsWithQuota(token.ID, fields, nil, token.Revision)
			if err == nil {
				successes.Add(1)
			} else if !errors.Is(err, ErrQuotaConflict) {
				t.Errorf("unexpected policy conflict: %v", err)
			}
		}()
	}
	wg.Wait()
	stored, err := GetTokenByID(token.ID)
	if err != nil || successes.Load() != 1 || ValidateTokenSmartPolicy(stored) != nil {
		t.Fatalf("concurrent edits combined incompatible policies: successes=%d token=%+v err=%v", successes.Load(), stored, err)
	}
}

func TestTokenSmartPolicyMigrationAndRestart(t *testing.T) {
	path := filepath.Join(t.TempDir(), "legacy-keys.db")
	if err := InitDB(path); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = CloseDB() })
	legacy := &Token{UserID: 1, Name: "legacy", ModelLimits: "alpha,beta", UsedQuota: 10}
	if err := InsertToken(legacy); err != nil {
		t.Fatal(err)
	}
	for _, column := range []string{"smart_access", "smart_candidates"} {
		if err := DB.Migrator().DropColumn(&Token{}, column); err != nil {
			t.Fatal(err)
		}
	}
	if err := CloseDB(); err != nil {
		t.Fatal(err)
	}
	if err := InitDB(path); err != nil {
		t.Fatal(err)
	}
	migrated, err := GetTokenByID(legacy.ID)
	if err != nil || migrated.SmartAccess != nil || migrated.SmartCandidates != "" || !migrated.AllowsSmartRouting() || migrated.UsedQuota != 10 {
		t.Fatalf("migration changed legacy authorization or usage: %+v %v", migrated, err)
	}
	if err := UpdateTokenFields(legacy.ID, map[string]any{"smart_access": false, "smart_candidates": "beta"}); err != nil {
		t.Fatal(err)
	}
	if err := CloseDB(); err != nil {
		t.Fatal(err)
	}
	if err := InitDB(path); err != nil {
		t.Fatal(err)
	}
	reopened, err := GetTokenByID(legacy.ID)
	if err != nil || reopened.SmartAccess == nil || *reopened.SmartAccess || reopened.SmartCandidates != "beta" || reopened.AllowsSmartRouting() || !reopened.AllowsModel("alpha") {
		t.Fatalf("restart lost independent smart policy: %+v %v", reopened, err)
	}
}
