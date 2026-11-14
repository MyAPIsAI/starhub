package model

import (
	"path/filepath"
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

func TestFailedCredentialMigrationRestoresLiveDatabase(t *testing.T) {
	path := filepath.Join(t.TempDir(), "invalid-credentials.db")
	if err := InitDB(path); err != nil {
		t.Fatal(err)
	}
	if err := DB.Table("channels").Create(map[string]any{"key": "enc:v2:missing:invalid:invalid", "name": "invalid"}).Error; err != nil {
		t.Fatal(err)
	}
	if err := CloseDB(); err != nil {
		t.Fatal(err)
	}
	newTestDB(t)
	previous := DB
	if err := InitDB(path); err == nil {
		t.Fatal("corrupt credentials were accepted")
	}
	if DB != previous {
		t.Fatal("failed migration replaced the active database")
	}
	var count int64
	if err := DB.Model(&Channel{}).Count(&count).Error; err != nil || count != 0 {
		t.Fatal("previous database is unusable", count, err)
	}
}

func TestInvalidNetworkDSNKeepsActiveDatabase(t *testing.T) {
	newTestDB(t)
	previous := DB
	for _, driver := range []string{"mysql", "postgres"} {
		err := InitDBWithConfig(DatabaseConfig{Driver: driver, DSN: "not-a-connection-string"})
		if err == nil {
			t.Fatal("invalid network DSN accepted", driver)
		}
		if DB != previous || CurrentDialect() != "sqlite" {
			t.Fatal("rejected backend changed the live database")
		}
	}
}

func TestInitDBPreservesSQLiteURIOptions(t *testing.T) {
	if err := InitDBWithConfig(DatabaseConfig{DSN: "file:p36-review-options?mode=memory&cache=shared"}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = CloseDB() })
	var timeout int
	if err := DB.Raw("PRAGMA busy_timeout").Scan(&timeout).Error; err != nil || timeout != 10000 {
		t.Fatal("required pragma was lost", timeout, err)
	}
	var file string
	if err := DB.Raw("SELECT file FROM pragma_database_list WHERE name='main'").Scan(&file).Error; err != nil || file != "" {
		t.Fatal("URI memory mode was lost", file, err)
	}
}
