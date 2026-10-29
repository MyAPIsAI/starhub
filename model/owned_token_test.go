package model

import (
	"errors"
	"gorm.io/gorm"
	"sync"
	"testing"
)

func TestOwnedTokenMetadataEditPreservesLegacyAutoPolicy(t *testing.T) {
	newTestDB(t)
	u := &User{Username: "legacy-auto-owner", Status: 1, AffCode: "legacy-auto"}
	if err := CreateUser(u); err != nil {
		t.Fatal(err)
	}
	token := &Token{UserID: u.ID, Name: "Automatic", Key: "synthetic-auto", Status: 1,
		Group: "auto", AutoGroups: "default,legacy-group", ModelLimits: "legacy-model",
		ExpiredTime: -1, UsedQuota: 20000000, RemainQuota: 30000000}
	if err := InsertToken(token); err != nil {
		t.Fatal(err)
	}
	choices, err := ReadOwnedTokenChoices(u.ID)
	if err != nil {
		t.Fatal(err)
	}
	updated, _, err := MutateOwnedToken(u.ID, "metadata-edit", "update", token.ID,
		TokenManagementRevision(token), choices.Revision,
		map[string]any{"name": "Renamed automatic key", "allow_ips": "127.0.0.1", "remain_quota": int64(40000000)})
	if err != nil {
		t.Fatal(err)
	}
	if updated.Group != token.Group || updated.AutoGroups != token.AutoGroups || updated.ModelLimits != token.ModelLimits || updated.UsedQuota != token.UsedQuota || updated.RemainQuota != 40000000 {
		t.Fatalf("metadata edit replaced the original routing or spending history: %+v", updated)
	}
}

func TestOwnedTokenActionsCompareSnapshotsAndPersistSecretFreeReceipts(t *testing.T) {
	newTestDB(t)
	u := &User{Username: "owned-keys", Status: 1, AffCode: "owned"}
	CreateUser(u)
	if err := InsertChannel(&Channel{Name: "owned", Type: 1, Status: 1, Key: "synthetic", BaseURL: "http://upstream.test", Models: "server-only", Group: "default"}); err != nil {
		t.Fatal(err)
	}
	choices, err := ReadOwnedTokenChoices(u.ID)
	if err != nil || len(choices.Choices) != 1 {
		t.Fatalf("choices: %+v %v", choices, err)
	}
	token, receipt, err := MutateOwnedToken(u.ID, "create", "create", 0, "", choices.Revision, map[string]any{"name": "Owned", "group": "default", "model_limits": "server-only", "unlimited_quota": true})
	if err != nil {
		t.Fatal(err)
	}
	if receipt.TokenID != token.ID || receipt.Revision != TokenManagementRevision(token) {
		t.Fatal("wrong create receipt")
	}
	if _, _, err := MutateOwnedToken(u.ID, "create", "create", 0, "", choices.Revision, map[string]any{"name": "Duplicate"}); !errors.Is(err, ErrOwnedTokenConflict) {
		t.Fatal("create replayed")
	}
	original := TokenManagementRevision(token)
	var group sync.WaitGroup
	wins := make(chan bool, 8)
	for i := 0; i < 8; i++ {
		group.Add(1)
		go func(i int) {
			defer group.Done()
			_, _, err := MutateOwnedToken(u.ID, string(rune('a'+i)), "regenerate", token.ID, original, choices.Revision, nil)
			wins <- err == nil
		}(i)
	}
	group.Wait()
	close(wins)
	count := 0
	for win := range wins {
		if win {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("regenerate winners=%d", count)
	}
	if _, _, err := MutateOwnedToken(u.ID, "stale", "delete", token.ID, original, choices.Revision, nil); !errors.Is(err, ErrOwnedTokenConflict) {
		t.Fatal("stale delete overwrote a regenerated credential")
	}
	fresh, _ := GetTokenByID(token.ID)
	DB.Callback().Create().Before("gorm:create").Register("fail-key-receipt", func(tx *gorm.DB) {
		if tx.Statement.Table == "owned_token_receipts" {
			tx.AddError(errors.New("injected"))
		}
	})
	if _, _, err := MutateOwnedToken(u.ID, "rollback", "delete", token.ID, TokenManagementRevision(fresh), choices.Revision, nil); err == nil {
		t.Fatal("receipt failure ignored")
	}
	DB.Callback().Create().Remove("fail-key-receipt")
	if _, err := GetTokenByID(token.ID); err != nil {
		t.Fatal("failed receipt did not roll back deletion")
	}
	if _, _, err := MutateOwnedToken(u.ID, "delete", "delete", token.ID, TokenManagementRevision(fresh), choices.Revision, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := ReadOwnedTokenReceipt(u.ID, "delete"); err != nil {
		t.Fatal(err)
	}
	other := &User{Username: "foreign", Status: 1, AffCode: "foreign"}
	CreateUser(other)
	if _, err := ReadOwnedTokenReceipt(other.ID, "delete"); err == nil {
		t.Fatal("foreign owner read receipt")
	}
}
func TestOwnedTokenActionsRejectForeignAndChangedChoices(t *testing.T) {
	newTestDB(t)
	u := &User{Username: "token-owner", Status: 1, AffCode: "owner"}
	CreateUser(u)
	InsertChannel(&Channel{Name: "owned", Type: 1, Status: 1, Key: "synthetic", Models: "server-only", Group: "default"})
	choices, _ := ReadOwnedTokenChoices(u.ID)
	if _, _, err := MutateOwnedToken(u.ID, "bad", "create", 0, "", choices.Revision, map[string]any{"name": "bad", "group": "default", "model_limits": "server-only-prefix"}); err == nil {
		t.Fatal("unknown model implicitly authorized")
	}
	token, _, err := MutateOwnedToken(u.ID, "ok", "create", 0, "", choices.Revision, map[string]any{"name": "ok", "group": "default", "model_limits": "server-only"})
	if err != nil {
		t.Fatal(err)
	}
	other := &User{Username: "root-foreign", Role: 100, Status: 1, AffCode: "root-foreign"}
	CreateUser(other)
	foreign, _ := ReadOwnedTokenChoices(other.ID)
	if _, _, err := MutateOwnedToken(other.ID, "foreign", "delete", token.ID, TokenManagementRevision(token), foreign.Revision, nil); !errors.Is(err, ErrOwnedTokenConflict) {
		t.Fatal("Root bypassed personal ownership")
	}
	if err := UpsertServiceGroup(&ServiceGroupConfig{GroupID: "default", Enabled: false}); err != nil {
		t.Fatal(err)
	}
	if _, _, err := MutateOwnedToken(u.ID, "changed", "update", token.ID, TokenManagementRevision(token), choices.Revision, map[string]any{"name": "changed"}); !errors.Is(err, ErrOwnedTokenConflict) {
		t.Fatal("changed choices overwritten")
	}
}
