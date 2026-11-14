package model

import (
	"strings"
	"testing"
)

func TestFailedInitializationKeepsActiveConnectionAndDialect(t *testing.T) {
	newTestDB(t)
	previous := DB
	if err := InitDBWithConfig(DatabaseConfig{Driver: "unknown"}); err == nil {
		t.Fatal("invalid driver accepted")
	}
	if DB != previous || CurrentDialect() != "sqlite" {
		t.Fatal("failed initialization changed the active connection or dialect")
	}
	var value int
	if err := DB.Raw("SELECT 1").Scan(&value).Error; err != nil || value != 1 {
		t.Fatal("active database is no longer usable", err)
	}
}

func TestUnreadyDatabaseDriversFailBeforeMigration(t *testing.T) {
	newTestDB(t)
	previous := DB
	for _, driver := range []string{"mysql", "postgres"} {
		err := InitDBWithConfig(DatabaseConfig{Driver: driver, DSN: "not-a-connection-string"})
		if err == nil || !strings.Contains(err.Error(), "not ready") {
			t.Fatal("unported backend reached connection or migration", driver, err)
		}
		if DB != previous || CurrentDialect() != "sqlite" {
			t.Fatal("rejected backend changed the live database")
		}
	}
}
