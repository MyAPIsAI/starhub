package model

import "testing"

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
