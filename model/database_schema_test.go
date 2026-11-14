package model

import (
	"testing"

	"gorm.io/gorm/clause"
)

func TestNetworkTextDefaultsPreserveRuntimeUpserts(t *testing.T) {
	for _, driver := range []string{"mysql", "postgres"} {
		t.Run(driver, func(t *testing.T) {
			db := networkProductionDB(t, driver)
			t.Cleanup(func() { _ = db.Migrator().DropTable(&Option{}) })
			for range 2 {
				if err := migrateDatabaseSchema(db, &Option{}); err != nil {
					t.Fatal(err)
				}
			}
			for _, value := range []string{"one", "two"} {
				if err := db.Clauses(clause.OnConflict{UpdateAll: true}).Create(&Option{Key: "test", Value: value}).Error; err != nil {
					t.Fatal(err)
				}
			}
			var option Option
			if err := db.First(&option).Error; err != nil || option.Value != "two" {
				t.Fatal("migration changed runtime upsert semantics", option, err)
			}
		})
	}
}
