package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/user"
	"starhub/model"
)

func main() {
	database := flag.String("db", "", "Existing SQLite database")
	owner := flag.Int64("user", 0, "Account ID to reconcile")
	date := flag.String("date", "", "Admission business date YYYY-MM-DD")
	zone := flag.String("timezone", "UTC", "Original admission timezone")
	region := flag.String("region", "", "Original admission region ID")
	legacy := flag.Bool("legacy", false, "Reconcile pre-calendar UTC records")
	reason := flag.String("reason", "", "Required audit reason")
	flag.Parse()
	if *database == "" || *owner <= 0 || *reason == "" || (*legacy && (*zone != "UTC" || *region != "")) {
		fail("db, user, date, original calendar and reason are required")
	}
	if _, err := os.Stat(*database); err != nil {
		fail("existing database required")
	}
	identity, err := user.Current()
	if err != nil {
		fail("operator identity unavailable")
	}
	day, err := model.BusinessDayForDate(*date, *zone, *region, 0)
	if err != nil {
		fail("invalid original business calendar")
	}
	if *legacy {
		day.BusinessCalendarVersion = 0
	}
	if err := model.InitDB(*database); err != nil {
		fail("database verification failed")
	}
	defer model.CloseDB()
	record, _, err := model.ReconcileBusinessDay(*owner, day, model.AuditLog{Username: identity.Username, Reason: *reason})
	if err != nil {
		fail("reconciliation failed; no balances were changed")
	}
	if err := json.NewEncoder(os.Stdout).Encode(record); err != nil {
		fail("report encoding failed")
	}
}

func fail(message string) { fmt.Fprintln(os.Stderr, message); os.Exit(1) }
