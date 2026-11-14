package config

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestDatabaseConfigResolvesDriverAndDSNOverridesTogether(t *testing.T) {
	for _, tc := range []struct {
		name, yaml, driver, dsn, path string
		env                           map[string]string
		invalid                       bool
	}{
		{name: "defaults", driver: "sqlite", dsn: "starhub.db", path: "starhub.db"},
		{name: "explicit SQLite DSN beats legacy path", yaml: "  sqlite_path: legacy.db\n  dsn: explicit.db\n", driver: "sqlite", dsn: "explicit.db", path: "relative"},
		{name: "environment path beats file DSN", yaml: "  dsn: explicit.db\n", env: map[string]string{"STARHUB_DB": "environment.db"}, driver: "sqlite", dsn: "environment.db"},
		{name: "environment DSN beats environment path", env: map[string]string{"STARHUB_DB": "legacy.db", "STARHUB_DB_DSN": "explicit.db"}, driver: "sqlite", dsn: "explicit.db"},
		{name: "network driver requires DSN", env: map[string]string{"STARHUB_DB_DRIVER": "mysql"}, invalid: true},
		{name: "network driver does not reuse SQLite path", yaml: "  sqlite_path: legacy.db\n", env: map[string]string{"STARHUB_DB_DRIVER": "postgres"}, invalid: true},
		{name: "driver switch does not reuse old network DSN", yaml: "  driver: mysql\n  dsn: mysql-connection\n", env: map[string]string{"STARHUB_DB_DRIVER": "postgres"}, invalid: true},
		{name: "driver switch to SQLite uses SQLite path", yaml: "  driver: mysql\n  dsn: mysql-connection\n", env: map[string]string{"STARHUB_DB_DRIVER": "sqlite", "STARHUB_DB": "chosen.db"}, driver: "sqlite", dsn: "chosen.db"},
		{name: "network environment overrides", yaml: "  dsn: explicit.db\n", env: map[string]string{"STARHUB_DB_DRIVER": "mysql", "STARHUB_DB_DSN": "network-connection"}, driver: "mysql", dsn: "network-connection"},
		{name: "empty DSN", yaml: "  dsn: ''\n", invalid: true},
		{name: "negative pool size", yaml: "  max_open_conns: -1\n", invalid: true},
		{name: "negative idle size", yaml: "  max_idle_conns: -1\n", invalid: true},
		{name: "invalid lifetime", yaml: "  conn_max_lifetime: invalid\n", invalid: true},
		{name: "negative lifetime", yaml: "  conn_max_lifetime: -1h\n", invalid: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var args []string
			var file string
			if tc.yaml != "" {
				file = writeStartupFile(t, "version: 1\ndatabase:\n"+tc.yaml)
				args = []string{"--config", file}
			}
			startup, err := LoadStartup(args, startupEnv(tc.env))
			if tc.invalid {
				if err == nil {
					t.Fatal("invalid database settings accepted")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			want := tc.dsn
			if tc.path == "relative" {
				want = filepath.Join(filepath.Dir(file), want)
			}
			if startup.Bootstrap.Database.Driver != tc.driver || startup.Bootstrap.Database.DSN != want {
				t.Fatalf("unexpected database configuration: %+v", startup.Bootstrap.Database)
			}
			if tc.driver == "sqlite" && startup.Bootstrap.DBPath != want {
				t.Fatal("legacy path differs from the actual SQLite connection")
			}
		})
	}
}

func TestDatabaseConfigDoesNotExposeDSNInValidationErrors(t *testing.T) {
	file := writeStartupFile(t, "version: 1\ndatabase:\n  driver: mysql\n  dsn: private-password\n  conn_max_lifetime: invalid\n")
	_, err := LoadStartup([]string{"--config", file}, startupEnv(nil))
	if err == nil || strings.Contains(err.Error(), "private-password") {
		t.Fatal("invalid lifetime was accepted or exposed the DSN", err)
	}
}
