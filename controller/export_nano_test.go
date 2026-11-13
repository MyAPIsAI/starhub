package controller

import (
	"encoding/csv"
	"starhub/common"
	"starhub/model"
	"strings"
	"testing"
	"time"
)

func TestNanoExportsPreserveSmallChargesAndCanonicalLedgerReferences(t *testing.T) {
	newTestDB(t)
	user := newUser(t, "nano-export", common.RoleUser, 1000)
	other := newUser(t, "other-nano-export", common.RoleUser, 0)
	token := mustToken(t, user.ID, 1000)
	if err := model.ReserveQuota(user.ID, token.ID, false, 2, "n2:export"); err != nil {
		t.Fatal(err)
	}
	if err := model.SettleQuota(user.ID, token.ID, false, -1, 1, &model.Log{QuotaReservationID: "n2:export", AmountUnit: common.MoneyUnitNano}); err != nil {
		t.Fatal(err)
	}
	if err := model.InsertLog(&model.Log{UserID: other.ID, Quota: 999, TokenName: "private-other-export"}); err != nil {
		t.Fatal(err)
	}
	for _, kind := range []string{"requests", "usage", "money_ledger"} {
		w := call(CreateExport, testRequest{method: "POST", target: "/api/export", userID: user.ID, role: user.Role, body: `{"type":"` + kind + `","range_days":31}`})
		expectStatus(t, w, 200)
		var record model.ExportRecord
		decodeData(t, w, &record)
		stored, err := model.GetExport(record.ID)
		if err != nil {
			t.Fatal(err)
		}
		rows, err := csv.NewReader(strings.NewReader(string(stored.Content))).ReadAll()
		if err != nil || len(rows) < 2 {
			t.Fatalf("empty exact export: %v", err)
		}
		if strings.Contains(string(stored.Content), "private-other-export") {
			t.Fatal("export included another account")
		}
		header := map[string]int{}
		for i, label := range rows[0] {
			header[label] = i
		}
		if kind == "money_ledger" {
			if rows[1][header["Spent Delta (nano-USD)"]] != "1" || rows[1][header["Operation ID"]] != "n2:export:settle" || rows[1][header["Usage Log ID"]] == "0" {
				t.Fatal("canonical audit references were lost")
			}
		} else if rows[1][header["Amount (nano-USD)"]] != "1" {
			t.Fatal("export rounded the charge to zero")
		}
	}
	if _, _, err := buildMoneyLedgerExport(user.ID, time.Now().Add(-time.Hour)); err != nil {
		t.Fatal(err)
	}
}
