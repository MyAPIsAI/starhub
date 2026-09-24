package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func startupEnv(values map[string]string) func(string) string {
	return func(key string) string { return values[key] }
}

func writeStartupFile(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.yml")
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestLoadStartupWithoutFilePreservesDefaultsAndEnvironment(t *testing.T) {
	startup, err := LoadStartup(nil, startupEnv(map[string]string{
		"STARHUB_PORT":                  "8080",
		"STARHUB_DB":                    "custom.db",
		"STARHUB_ROOT_PASSWORD":         "secret",
		"STARHUB_TASK_POLL_INTERVAL":    "0",
		"STARHUB_CHANNEL_TEST_INTERVAL": "120",
		"STARHUB_COOKIE_SECURE":         "true",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if startup.Bootstrap.Port != "8080" || startup.Bootstrap.DBPath != "custom.db" || startup.Bootstrap.RootPassword != "secret" || !startup.Bootstrap.ForceSecureCookie {
		t.Fatalf("unexpected bootstrap: %+v", startup.Bootstrap)
	}
	if startup.InitialOptions[OptionTaskPollInterval] != "0" || startup.InitialOptions[OptionChannelTestInterval] != "120" || startup.InitialOptions[OptionCookieSecure] != "true" {
		t.Fatalf("unexpected options: %+v", startup.InitialOptions)
	}
	defaults, err := LoadStartup(nil, startupEnv(nil))
	if err != nil || defaults.Bootstrap.Port != "3000" || defaults.Bootstrap.DBPath != "starhub.db" || len(defaults.InitialOptions) != 0 {
		t.Fatalf("unexpected defaults: %+v, %v", defaults, err)
	}
}

func TestLoadStartupFilePrecedenceAndRelativePath(t *testing.T) {
	file := writeStartupFile(t, "version: 1\nserver:\n  port: 3100\ndatabase:\n  sqlite_path: data/starhub.db\nsecurity:\n  force_secure_cookie: true\ninitial_options:\n  channel_test_interval: 90\n  task_poll_interval: 0\n  cookie_secure: false\n  ip_rate_limit: 400\n")
	startup, err := LoadStartup([]string{"--config", file}, startupEnv(map[string]string{
		"STARHUB_CONFIG":                filepath.Join(t.TempDir(), "missing.yml"),
		"STARHUB_PORT":                  "3200",
		"STARHUB_CHANNEL_TEST_INTERVAL": "30",
		"STARHUB_COOKIE_SECURE":         "false",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if startup.ConfigPath != file || startup.Bootstrap.Port != "3200" || startup.Bootstrap.DBPath != filepath.Join(filepath.Dir(file), "data", "starhub.db") || !startup.Bootstrap.ForceSecureCookie {
		t.Fatalf("unexpected bootstrap: %+v", startup)
	}
	if startup.InitialOptions[OptionChannelTestInterval] != "30" || startup.InitialOptions[OptionTaskPollInterval] != "0" || startup.InitialOptions[OptionCookieSecure] != "false" || startup.InitialOptions[OptionIPRateLimit] != "400" {
		t.Fatalf("unexpected options: %+v", startup.InitialOptions)
	}
	fromEnv, err := LoadStartup(nil, startupEnv(map[string]string{"STARHUB_CONFIG": file}))
	if err != nil || fromEnv.Bootstrap.Port != "3100" {
		t.Fatalf("environment file selection failed: %+v, %v", fromEnv, err)
	}
	fromEquals, err := LoadStartup([]string{"--config=" + file}, startupEnv(nil))
	if err != nil || fromEquals.ConfigPath != file {
		t.Fatalf("equals flag failed: %+v, %v", fromEquals, err)
	}
}

func TestExampleStartupConfigParses(t *testing.T) {
	startup, err := LoadStartup([]string{"--config", filepath.Join("..", "config.example.yml")}, startupEnv(nil))
	if err != nil {
		t.Fatal(err)
	}
	if len(startup.InitialOptions) != len(OptionKeys()) {
		t.Fatalf("example covers %d of %d runtime options", len(startup.InitialOptions), len(OptionKeys()))
	}
	if startup.Bootstrap.DBPath != filepath.Join(filepath.Dir(startup.ConfigPath), "starhub.db") {
		t.Fatalf("unexpected example database path: %s", startup.Bootstrap.DBPath)
	}
}

func TestLoadStartupRejectsInvalidFilesAndValues(t *testing.T) {
	cases := []struct {
		name, content, want string
	}{
		{"unknown key", "version: 1\nserver:\n  other: 3000\n", "field other not found"},
		{"duplicate key", "version: 1\nversion: 1\n", "already defined"},
		{"wrong version", "version: 2\n", "unsupported config version"},
		{"multiple documents", "version: 1\n---\nversion: 1\n", "exactly one YAML document"},
		{"invalid port", "version: 1\nserver:\n  port: 65536\n", "server port"},
		{"empty path", "version: 1\ndatabase:\n  sqlite_path: ''\n", "sqlite_path"},
		{"invalid limit", "version: 1\ninitial_options:\n  ip_rate_limit: 0\n", "ip_rate_limit"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			path := writeStartupFile(t, tc.content)
			if _, err := LoadStartup([]string{"--config", path}, startupEnv(nil)); err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("want error containing %q, got %v", tc.want, err)
			}
		})
	}
	if _, err := LoadStartup([]string{"--config"}, startupEnv(nil)); err == nil {
		t.Fatal("missing flag value accepted")
	}
	if _, err := LoadStartup([]string{"--unexpected"}, startupEnv(nil)); err == nil {
		t.Fatal("unknown flag accepted")
	}
	if _, err := LoadStartup(nil, startupEnv(map[string]string{"STARHUB_CONFIG": filepath.Join(t.TempDir(), "missing.yml")})); err == nil {
		t.Fatal("missing config file accepted")
	}
	if _, err := LoadStartup(nil, startupEnv(map[string]string{"STARHUB_PORT": "0"})); err == nil {
		t.Fatal("invalid environment port accepted")
	}
	if _, err := LoadStartup(nil, startupEnv(map[string]string{"STARHUB_COOKIE_SECURE": "maybe"})); err == nil {
		t.Fatal("invalid environment cookie value accepted")
	}
	tooLarge := writeStartupFile(t, "version: 1\n#"+strings.Repeat("x", 1<<20))
	if _, err := LoadStartup([]string{"--config", tooLarge}, startupEnv(nil)); err == nil || !strings.Contains(err.Error(), "exceeds 1 MiB") {
		t.Fatalf("oversized config was not rejected: %v", err)
	}
}

func TestApplyStartupValidatesBeforeChangingRuntime(t *testing.T) {
	before := Current()
	beforeForce := ForceSecureCookie()
	previous := make(map[string]string)
	for _, key := range OptionKeys() {
		value, err := SnapshotOption(key)
		if err != nil {
			t.Fatal(err)
		}
		previous[key] = value
	}
	t.Cleanup(func() {
		for key, value := range previous {
			_ = ApplyOption(key, value)
		}
		forceSecureCookie.Store(beforeForce)
	})
	if err := ApplyStartup(Startup{InitialOptions: map[string]string{OptionTaskPollInterval: "2", OptionIPRateLimit: "0"}}); err == nil {
		t.Fatal("invalid startup options accepted")
	}
	if Current() != before {
		t.Fatal("invalid startup partially changed runtime")
	}
	if err := ApplyStartup(Startup{Bootstrap: Bootstrap{ForceSecureCookie: true}, InitialOptions: map[string]string{OptionTaskPollInterval: "0"}}); err != nil {
		t.Fatal(err)
	}
	if Current().TaskPollInterval != 0 || !ForceSecureCookie() {
		t.Fatal("valid startup was not applied")
	}
}
