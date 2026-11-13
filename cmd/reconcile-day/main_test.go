package main

import (
	"encoding/json"
	"flag"
	"os"
	"os/exec"
	"path/filepath"
	"starhub/common"
	"starhub/model"
	"strconv"
	"testing"
)

func TestHistoricalReconciliationCommandIsRepeatableAndAudited(t *testing.T) {
	if os.Getenv("STARHUB_RECONCILE_TEST_HELPER") == "1" {
		for i, arg := range os.Args {
			if arg == "--" {
				os.Args = append([]string{os.Args[0]}, os.Args[i+1:]...)
				break
			}
		}
		flag.CommandLine = flag.NewFlagSet("reconcile-day", flag.ExitOnError)
		main()
		return
	}
	t.Setenv("STARHUB_CRYPTO_SECRET", "reconcile-command-fixture-only")
	database := filepath.Join(t.TempDir(), "reconcile.db")
	if err := model.InitDB(database); err != nil {
		t.Fatal(err)
	}
	user := &model.User{Username: "historical-command", Status: 1, Quota: 100, AffCode: common.GenerateAffCode()}
	if err := model.CreateUser(user); err != nil {
		t.Fatal(err)
	}
	if _, err := model.ReadMoneyAccount(user.ID); err != nil {
		t.Fatal(err)
	}
	if err := model.CloseDB(); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		command := exec.Command(os.Args[0], "-test.run=^TestHistoricalReconciliationCommandIsRepeatableAndAudited$", "--", "-db", database, "-user", strconv.FormatInt(user.ID, 10), "-date", "2026-11-10", "-timezone", "Asia/Dubai", "-region", "ae", "-reason", "Verify original admission calendar")
		command.Env = append(os.Environ(), "STARHUB_RECONCILE_TEST_HELPER=1")
		output, err := command.CombinedOutput()
		if err != nil {
			t.Fatalf("command failed: %v %s", err, output)
		}
		// The helper test process appends its PASS line after the command output.
		var report model.ReconciliationRecord
		if err := json.Unmarshal(firstJSONLine(output), &report); err != nil || report.Revision != 1 || report.State != "matched" || report.BusinessTimezone != "Asia/Dubai" {
			t.Fatalf("invalid historical report: %s %v", output, err)
		}
	}
	if err := model.InitDB(database); err != nil {
		t.Fatal(err)
	}
	defer model.CloseDB()
	var reports, audits int64
	model.DB.Model(&model.ReconciliationRecord{}).Count(&reports)
	model.DB.Model(&model.AuditLog{}).Where("method = ? AND reason = ?", "LOCAL", "Verify original admission calendar").Count(&audits)
	if reports != 1 || audits != 2 {
		t.Fatalf("reports=%d audits=%d", reports, audits)
	}
	after, err := model.GetUserByID(user.ID)
	if err != nil || after.Quota != 100 || after.UsedQuota != 0 {
		t.Fatal("command changed balances", err)
	}
}

func firstJSONLine(output []byte) []byte {
	for i, b := range output {
		if b == '\n' {
			return output[:i]
		}
	}
	return output
}
