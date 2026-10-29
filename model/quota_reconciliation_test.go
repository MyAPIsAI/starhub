package model

import (
	"bytes"
	"log"
	"strings"
	"sync"
	"testing"
)

func TestSettlementReconcilesDifferentBalances(t *testing.T) {
	for _, tc := range []struct {
		name                  string
		userQuota, tokenQuota int64
		unlimited, omitLog    bool
		charged               int64
	}{
		{"token short", 1000, 100, false, false, 100},
		{"user short", 100, 1000, false, false, 100},
		{"both sufficient", 1000, 1000, false, false, 130},
		{"unlimited token", 100, 0, true, false, 100},
		{"missing log", 100, 100, false, true, 100},
	} {
		t.Run(tc.name, func(t *testing.T) {
			newTestDB(t)
			user, token := newBillingUser(t, tc.userQuota)
			if err := DB.Model(token).Updates(map[string]any{"remain_quota": tc.tokenQuota, "unlimited_quota": tc.unlimited}).Error; err != nil {
				t.Fatal(err)
			}
			channel := &Channel{Name: "billing", Key: "k", Models: "m", Group: "default", Status: 1}
			if err := InsertChannel(channel); err != nil {
				t.Fatal(err)
			}
			if err := ReserveQuota(user.ID, token.ID, tc.unlimited, 80); err != nil {
				t.Fatal(err)
			}
			entry := &Log{ChannelID: channel.ID, Model: "m", Quota: 130}
			if tc.omitLog {
				entry = nil
			}
			var output bytes.Buffer
			oldWriter := log.Writer()
			log.SetOutput(&output)
			t.Cleanup(func() { log.SetOutput(oldWriter) })
			if err := SettleQuota(user.ID, token.ID, tc.unlimited, 50, 130, entry); err != nil {
				t.Fatal(err)
			}
			gotUser, err := GetUserByID(user.ID)
			if err != nil {
				t.Fatal(err)
			}
			gotToken, err := GetTokenByID(token.ID)
			if err != nil {
				t.Fatal(err)
			}
			if gotUser.Quota != tc.userQuota-tc.charged || gotUser.UsedQuota != tc.charged {
				t.Fatalf("user balance=%d used=%d", gotUser.Quota, gotUser.UsedQuota)
			}
			wantTokenQuota, wantTokenUsed := tc.tokenQuota-tc.charged, tc.charged
			if tc.unlimited {
				wantTokenQuota = tc.tokenQuota
			}
			if gotToken.RemainQuota != wantTokenQuota || gotToken.UsedQuota != wantTokenUsed {
				t.Fatalf("token balance=%d used=%d", gotToken.RemainQuota, gotToken.UsedQuota)
			}
			logs, count, err := ListLogs(user.ID, 1, 10, "")
			if err != nil || count != 1 {
				t.Fatalf("logs count=%d error=%v", count, err)
			}
			if logs[0].Quota != tc.charged || logs[0].UnfundedQuota != 130-tc.charged {
				t.Fatalf("incorrect persisted settlement: %+v", logs[0])
			}
			if entry != nil && (entry.ID != logs[0].ID || entry.Quota != tc.charged) {
				t.Fatalf("caller must receive committed charge: %+v", entry)
			}
			if entry != nil {
				gotChannel, err := GetChannelByID(channel.ID)
				if err != nil || gotChannel.UsedQuota != tc.charged {
					t.Fatalf("channel=%+v error=%v", gotChannel, err)
				}
			}
			daily, err := SummarizeLogsByDay(user.ID, 0)
			if err != nil || len(daily) != 1 || daily[0].Quota != tc.charged || daily[0].UnfundedQuota != 130-tc.charged {
				t.Fatalf("daily totals=%+v error=%v", daily, err)
			}
			if strings.Contains(output.String(), "[quota_shortfall]") != (tc.charged < 130) {
				t.Fatalf("unexpected audit output: %s", output.String())
			}
		})
	}
}

func TestSettlementRollbackPreservesBalancesAndLog(t *testing.T) {
	newTestDB(t)
	user, token := newBillingUser(t, 100)
	if err := ReserveQuota(user.ID, token.ID, false, 80); err != nil {
		t.Fatal(err)
	}
	existing := &Log{UserID: user.ID}
	if err := InsertLog(existing); err != nil {
		t.Fatal(err)
	}
	entry := &Log{ID: existing.ID, Quota: 130}
	before := *entry
	if err := SettleQuota(user.ID, token.ID, false, 50, 130, entry); err == nil {
		t.Fatal("duplicate log ID should roll back the transaction")
	}
	gotUser, _ := GetUserByID(user.ID)
	gotToken, _ := GetTokenByID(token.ID)
	if gotUser.Quota != 20 || gotToken.RemainQuota != 20 || gotUser.UsedQuota != 0 || gotToken.UsedQuota != 0 {
		t.Fatalf("failed settlement changed balances: user=%+v token=%+v", gotUser, gotToken)
	}
	if *entry != before {
		t.Fatalf("failed settlement mutated caller log: %+v", entry)
	}
}

func TestConcurrentSettlementsReconcileShortfalls(t *testing.T) {
	newTestDB(t)
	user, token := newBillingUser(t, 100)
	const workers = 32
	for i := 0; i < workers; i++ {
		if err := ReserveQuota(user.ID, token.ID, false, 1); err != nil {
			t.Fatal(err)
		}
	}
	errors := make(chan error, workers)
	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			errors <- SettleQuota(user.ID, token.ID, false, 9, 10, &Log{})
		}()
	}
	wg.Wait()
	close(errors)
	for err := range errors {
		if err != nil {
			t.Fatal(err)
		}
	}
	gotUser, _ := GetUserByID(user.ID)
	gotToken, _ := GetTokenByID(token.ID)
	if gotUser.Quota != 0 || gotToken.RemainQuota != 0 || gotUser.UsedQuota != 100 || gotToken.UsedQuota != 100 {
		t.Fatalf("concurrent balances: user=%+v token=%+v", gotUser, gotToken)
	}
	daily, err := SummarizeLogsByDay(user.ID, 0)
	if err != nil || len(daily) != 1 || daily[0].Requests != workers || daily[0].Quota != 100 || daily[0].UnfundedQuota != 220 {
		t.Fatalf("concurrent totals=%+v error=%v", daily, err)
	}
}
