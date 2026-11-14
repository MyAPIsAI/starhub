package model

import (
	"sync"
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/schema"
)

func TestProductionPayloadTypesCoverApplicationBudgets(t *testing.T) {
	for _, value := range []any{&ProductionAttemptRecord{}, &ProductionTelemetryMinute{}, &RoutingSnapshotRecord{}} {
		s, err := schema.Parse(value, &sync.Map{}, schema.NamingStrategy{})
		if err != nil {
			t.Fatal(err)
		}
		for _, dialect := range []gorm.Dialector{sqlite.Open(":memory:"), mysql.Open(""), postgres.Open("")} {
			want := "text"
			if dialect.Name() == "mysql" {
				want = "longtext"
			}
			if got := dialect.DataTypeOf(s.LookUpField("Payload")); got != want {
				t.Errorf("%s %s payload type = %s, want %s", dialect.Name(), s.Name, got, want)
			}
		}
	}
}
