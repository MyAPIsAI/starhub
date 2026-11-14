package model

import (
	"os"
	"testing"
)

func TestNetworkApplicationInitialization(t *testing.T) {
	for _, driver := range []string{"mysql", "postgres"} {
		t.Run(driver, func(t *testing.T) {
			fixture := networkProductionDB(t, driver)
			t.Cleanup(func() {
				if DB != fixture {
					_ = CloseDB()
					DB = fixture
				}
				tables, err := fixture.Migrator().GetTables()
				if err != nil {
					t.Error(err)
					return
				}
				for _, table := range tables {
					if err := fixture.Migrator().DropTable(table); err != nil {
						t.Error(err)
					}
				}
			})
			env := "STARHUB_TEST_MYSQL_DSN"
			if driver == "postgres" {
				env = "STARHUB_TEST_POSTGRES_DSN"
			}
			cfg := DatabaseConfig{Driver: driver, DSN: os.Getenv(env), MaxOpenConns: 8, MaxIdleConns: 2, ConnMaxLifetime: "1m"}
			if err := initDBWithConfig(cfg); err != nil {
				t.Fatal("full migration", err)
			}
			if err := Seed("p37-local-test"); err != nil {
				t.Fatal("application seed", err)
			}
			if err := CloseDB(); err != nil {
				t.Fatal(err)
			}
			if err := initDBWithConfig(cfg); err != nil {
				t.Fatal("repeated application migration", err)
			}
		})
	}
}
