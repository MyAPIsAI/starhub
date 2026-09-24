package model

import (
	"os"
	"path/filepath"
	"testing"

	"starhub/config"
)

func TestStartupOptionsSeedOnlyMissingDatabaseKeys(t *testing.T) {
	previous := make(map[string]string)
	for _, key := range config.OptionKeys() {
		value, err := config.SnapshotOption(key)
		if err != nil {
			t.Fatal(err)
		}
		previous[key] = value
	}
	t.Cleanup(func() {
		for key, value := range previous {
			_ = config.ApplyOption(key, value)
		}
	})
	newTestDB(t)
	path := filepath.Join(t.TempDir(), "config.yml")
	if err := os.WriteFile(path, []byte("version: 1\ninitial_options:\n  task_poll_interval: 12\n  ip_rate_limit: 300\n"), 0600); err != nil {
		t.Fatal(err)
	}
	startup, err := config.LoadStartup([]string{"--config", path}, func(string) string { return "" })
	if err != nil {
		t.Fatal(err)
	}
	if err := config.ApplyStartup(startup); err != nil {
		t.Fatal(err)
	}
	if err := SeedOptions(); err != nil {
		t.Fatal(err)
	}
	option, found, err := GetOption(config.OptionTaskPollInterval)
	if err != nil || !found || option != "12" {
		t.Fatalf("new database default = %q, %t, %v", option, found, err)
	}
	if err := UpsertOption(config.OptionTaskPollInterval, "45"); err != nil {
		t.Fatal(err)
	}
	if err := SeedOptions(); err != nil {
		t.Fatal(err)
	}
	option, found, err = GetOption(config.OptionTaskPollInterval)
	if err != nil || !found || option != "45" {
		t.Fatalf("existing database option overwritten: %q, %t, %v", option, found, err)
	}
}
