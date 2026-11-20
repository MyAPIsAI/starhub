package model

import (
	"github.com/glebarez/sqlite"
	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDateFromUnixUsesUTCWithoutSessionTimezone(t *testing.T) {
	previous := DB
	t.Cleanup(func() { DB = previous })
	for _, driver := range []string{"sqlite", "postgres", "mysql"} {
		var dialector gorm.Dialector = sqlite.Open(":memory:")
		if driver == "mysql" {
			dialector = mysql.Open("")
		}
		if driver == "postgres" {
			dialector = postgres.Open("")
		}
		DB = &gorm.DB{Config: &gorm.Config{Dialector: dialector}}
		expression := DateFromUnix(admissionDayColumn)
		if !strings.Contains(expression, admissionDayColumn) {
			t.Fatal("admission fallback was lost", expression)
		}
		switch driver {
		case "postgres":
			if !strings.Contains(expression, "AT TIME ZONE 'UTC'") {
				t.Fatal("session timezone can change the business date", expression)
			}
		case "mysql":
			if strings.Contains(expression, "FROM_UNIXTIME") || !strings.Contains(expression, "'1970-01-01 00:00:00'") {
				t.Fatal("date must use a UTC datetime epoch", expression)
			}
		case "sqlite":
			if expression != "date("+admissionDayColumn+", 'unixepoch')" {
				t.Fatal("SQLite date behavior changed", expression)
			}
		}
	}
}

// TestNoUnguardedSQLiteSyntaxInProduction is a regression guard for P27
// chapter 15 risk 8. It scans non-test Go files in the model, service and
// controller packages for SQLite-specific syntax that is not routed through
// a dialect-aware helper. Strftime and unguarded date(...,'unixepoch') calls
// break MySQL and PostgreSQL. GROUP_CONCAT is acceptable only when a dialect
// branch exists in the same function.
func TestNoUnguardedSQLiteSyntaxInProduction(t *testing.T) {
	root := ".."
	packages := []string{"model", "service", "controller"}
	sqlitePatterns := []string{
		"strftime(",
		"date(', 'unixepoch')",
		"sqlite_master",
	}
	allowedFiles := map[string]bool{
		filepath.Join("model", "dialect.go"):                 true,
		filepath.Join("model", "database_indexes.go"):        true,
		filepath.Join("model", "database_backup.go"):         true,
		filepath.Join("model", "database_init_test.go"):      true,
		filepath.Join("model", "dialect_test.go"):            true,
		filepath.Join("model", "dialect_integration_test.go"): true,
		filepath.Join("model", "local_routing_write_test.go"): true,
		filepath.Join("model", "resource_binding_change_test.go"): true,
	}
	for _, pkg := range packages {
		dir := filepath.Join(root, pkg)
		entries, err := os.ReadDir(dir)
		if err != nil {
			t.Fatalf("could not read %s: %v", dir, err)
		}
		for _, entry := range entries {
			if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") {
				continue
			}
			relPath := filepath.Join(pkg, entry.Name())
			if strings.HasSuffix(entry.Name(), "_test.go") {
				continue
			}
			if allowedFiles[relPath] {
				continue
			}
			content, err := os.ReadFile(filepath.Join(dir, entry.Name()))
			if err != nil {
				t.Fatalf("could not read %s: %v", relPath, err)
			}
			source := string(content)
			for _, pattern := range sqlitePatterns {
				if strings.Contains(source, pattern) {
					t.Errorf("%s contains SQLite-specific syntax %q not routed through a dialect helper", relPath, pattern)
				}
			}
		}
	}
}
