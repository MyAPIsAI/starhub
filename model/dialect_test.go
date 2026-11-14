package model

import (
	"github.com/glebarez/sqlite"
	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
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
