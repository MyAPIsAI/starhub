package config

import (
	"bytes"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"gopkg.in/yaml.v3"
)

// DatabaseConfig describes how to connect to the database. Driver is
// "sqlite" (default), "mysql", or "postgres". DSN is the connection
// string; for sqlite it is the file path. Pool settings only apply to
// mysql and postgres.
type DatabaseConfig struct {
	Driver          string
	DSN             string
	MaxOpenConns    int
	MaxIdleConns    int
	ConnMaxLifetime string
}

type Bootstrap struct {
	BindAddress       string
	DBPath            string // backward-compatible sqlite path, also used for logging
	Database          DatabaseConfig
	Port              string
	RootPassword      string
	ForceSecureCookie bool
}

type Startup struct {
	Bootstrap      Bootstrap
	InitialOptions map[string]string
	ConfigPath     string
}

type fileConfig struct {
	Version int `yaml:"version"`
	Server  struct {
		Port        *int    `yaml:"port"`
		BindAddress *string `yaml:"bind_address"`
	} `yaml:"server"`
	Database struct {
		Driver          *string `yaml:"driver"`
		DSN             *string `yaml:"dsn"`
		SQLitePath      *string `yaml:"sqlite_path"`
		MaxOpenConns    *int    `yaml:"max_open_conns"`
		MaxIdleConns    *int    `yaml:"max_idle_conns"`
		ConnMaxLifetime *string `yaml:"conn_max_lifetime"`
	} `yaml:"database"`
	Security struct {
		ForceSecureCookie *bool `yaml:"force_secure_cookie"`
	} `yaml:"security"`
	InitialOptions struct {
		ChannelTestInterval *int64 `yaml:"channel_test_interval"`
		TaskPollInterval    *int64 `yaml:"task_poll_interval"`
		CookieSecure        *bool  `yaml:"cookie_secure"`
		IPRateLimit         *int64 `yaml:"ip_rate_limit"`
		UserRateLimit       *int64 `yaml:"user_rate_limit"`
		TokenRateLimit      *int64 `yaml:"token_rate_limit"`
		ModelRateLimit      *int64 `yaml:"model_rate_limit"`
	} `yaml:"initial_options"`
}

var forceSecureCookie atomic.Bool

func ForceSecureCookie() bool { return forceSecureCookie.Load() }

// LoadStartup resolves startup configuration without mutating live settings.
func LoadStartup(args []string, getenv func(string) string) (Startup, error) {
	path, err := configPath(args, getenv)
	if err != nil {
		return Startup{}, err
	}
	result := Startup{
		Bootstrap: Bootstrap{
			DBPath:   "starhub.db",
			Database: DatabaseConfig{Driver: "sqlite"},
			Port:     "3000", RootPassword: getenv("STARHUB_ROOT_PASSWORD"),
		},
		InitialOptions: make(map[string]string),
	}
	dsnDriver := "sqlite"
	if path != "" {
		absolute, err := filepath.Abs(path)
		if err != nil {
			return Startup{}, fmt.Errorf("resolve config path: %w", err)
		}
		result.ConfigPath = absolute
		file, err := os.Open(absolute)
		if err != nil {
			return Startup{}, fmt.Errorf("read config %s: %w", absolute, err)
		}
		defer file.Close()
		data, err := io.ReadAll(io.LimitReader(file, (1<<20)+1))
		if err != nil {
			return Startup{}, fmt.Errorf("read config %s: %w", absolute, err)
		}
		if len(data) > 1<<20 {
			return Startup{}, fmt.Errorf("config file exceeds 1 MiB")
		}
		var cfg fileConfig
		decoder := yaml.NewDecoder(bytes.NewReader(data))
		decoder.KnownFields(true)
		if err := decoder.Decode(&cfg); err != nil {
			return Startup{}, fmt.Errorf("parse config %s: %w", absolute, err)
		}
		var extra any
		if err := decoder.Decode(&extra); err != io.EOF {
			return Startup{}, fmt.Errorf("config must contain exactly one YAML document")
		}
		if cfg.Version != 1 {
			return Startup{}, fmt.Errorf("unsupported config version %d; expected 1", cfg.Version)
		}
		if cfg.Server.BindAddress != nil {
			result.Bootstrap.BindAddress = strings.TrimSpace(*cfg.Server.BindAddress)
		}
		if cfg.Server.Port != nil {
			result.Bootstrap.Port = strconv.Itoa(*cfg.Server.Port)
		}
		if cfg.Database.Driver != nil {
			result.Bootstrap.Database.Driver = strings.TrimSpace(*cfg.Database.Driver)
		}
		if cfg.Database.DSN != nil {
			if strings.TrimSpace(*cfg.Database.DSN) == "" {
				return Startup{}, fmt.Errorf("database.dsn must not be empty")
			}
			result.Bootstrap.Database.DSN = *cfg.Database.DSN
			dsnDriver = result.Bootstrap.Database.Driver
			if dsnDriver == "sqlite" && *cfg.Database.DSN != "" && *cfg.Database.DSN != ":memory:" && !strings.HasPrefix(*cfg.Database.DSN, "file:") && !filepath.IsAbs(*cfg.Database.DSN) {
				result.Bootstrap.Database.DSN = filepath.Join(filepath.Dir(absolute), *cfg.Database.DSN)
			}
		}
		if cfg.Database.SQLitePath != nil {
			if strings.TrimSpace(*cfg.Database.SQLitePath) == "" {
				return Startup{}, fmt.Errorf("database.sqlite_path must not be empty")
			}
			result.Bootstrap.DBPath = *cfg.Database.SQLitePath
			if !filepath.IsAbs(result.Bootstrap.DBPath) {
				result.Bootstrap.DBPath = filepath.Join(filepath.Dir(absolute), result.Bootstrap.DBPath)
			}
		}
		if cfg.Database.MaxOpenConns != nil {
			result.Bootstrap.Database.MaxOpenConns = *cfg.Database.MaxOpenConns
		}
		if cfg.Database.MaxIdleConns != nil {
			result.Bootstrap.Database.MaxIdleConns = *cfg.Database.MaxIdleConns
		}
		if cfg.Database.ConnMaxLifetime != nil {
			result.Bootstrap.Database.ConnMaxLifetime = *cfg.Database.ConnMaxLifetime
		}
		if cfg.Security.ForceSecureCookie != nil {
			result.Bootstrap.ForceSecureCookie = *cfg.Security.ForceSecureCookie
		}
		addInt := func(key string, value *int64) {
			if value != nil {
				result.InitialOptions[key] = strconv.FormatInt(*value, 10)
			}
		}
		addInt(OptionChannelTestInterval, cfg.InitialOptions.ChannelTestInterval)
		addInt(OptionTaskPollInterval, cfg.InitialOptions.TaskPollInterval)
		addInt(OptionIPRateLimit, cfg.InitialOptions.IPRateLimit)
		addInt(OptionUserRateLimit, cfg.InitialOptions.UserRateLimit)
		addInt(OptionTokenRateLimit, cfg.InitialOptions.TokenRateLimit)
		addInt(OptionModelRateLimit, cfg.InitialOptions.ModelRateLimit)
		if cfg.InitialOptions.CookieSecure != nil {
			result.InitialOptions[OptionCookieSecure] = strconv.FormatBool(*cfg.InitialOptions.CookieSecure)
		}
	}
	if value := getenv("STARHUB_BIND_ADDRESS"); value != "" {
		result.Bootstrap.BindAddress = strings.TrimSpace(value)
	}
	if value := getenv("STARHUB_PORT"); value != "" {
		result.Bootstrap.Port = value
	}
	if value := getenv("STARHUB_DB_DRIVER"); value != "" {
		result.Bootstrap.Database.Driver = strings.TrimSpace(value)
	}
	// A file DSN belongs to its file driver, not an environment override.
	if result.Bootstrap.Database.Driver != dsnDriver {
		result.Bootstrap.Database.DSN = ""
	}
	if value := getenv("STARHUB_DB"); value != "" {
		result.Bootstrap.DBPath = value
		if result.Bootstrap.Database.Driver == "sqlite" {
			result.Bootstrap.Database.DSN = value
		}
	}
	if value := getenv("STARHUB_DB_DSN"); value != "" {
		result.Bootstrap.Database.DSN = value
	}
	if result.Bootstrap.Database.Driver == "sqlite" {
		if result.Bootstrap.Database.DSN == "" {
			result.Bootstrap.Database.DSN = result.Bootstrap.DBPath
		}
		result.Bootstrap.DBPath = result.Bootstrap.Database.DSN
	}
	for env, key := range map[string]string{
		"STARHUB_CHANNEL_TEST_INTERVAL": OptionChannelTestInterval,
		"STARHUB_TASK_POLL_INTERVAL":    OptionTaskPollInterval,
		"STARHUB_COOKIE_SECURE":         OptionCookieSecure,
	} {
		if value := getenv(env); value != "" {
			result.InitialOptions[key] = value
		}
	}
	if value := getenv("STARHUB_COOKIE_SECURE"); value != "" {
		secure, err := strconv.ParseBool(value)
		if err != nil {
			return Startup{}, fmt.Errorf("STARHUB_COOKIE_SECURE: %w", err)
		}
		result.Bootstrap.ForceSecureCookie = result.Bootstrap.ForceSecureCookie || secure
	}
	if result.Bootstrap.BindAddress != "" && net.ParseIP(result.Bootstrap.BindAddress) == nil {
		return Startup{}, fmt.Errorf("server bind_address must be an IP address")
	}
	port, err := strconv.Atoi(result.Bootstrap.Port)
	if err != nil || port < 1 || port > 65535 {
		return Startup{}, fmt.Errorf("server port must be in [1,65535]")
	}
	if strings.TrimSpace(result.Bootstrap.DBPath) == "" {
		return Startup{}, fmt.Errorf("database path must not be empty")
	}
	switch result.Bootstrap.Database.Driver {
	case "sqlite", "mysql", "postgres":
	default:
		return Startup{}, fmt.Errorf("unsupported database driver: %s", result.Bootstrap.Database.Driver)
	}
	if strings.TrimSpace(result.Bootstrap.Database.DSN) == "" {
		return Startup{}, fmt.Errorf("database dsn must not be empty")
	}
	if result.Bootstrap.Database.MaxOpenConns < 0 || result.Bootstrap.Database.MaxIdleConns < 0 {
		return Startup{}, fmt.Errorf("database connection limits must not be negative")
	}
	if value := result.Bootstrap.Database.ConnMaxLifetime; value != "" {
		duration, err := time.ParseDuration(value)
		if err != nil || duration < 0 {
			return Startup{}, fmt.Errorf("database.conn_max_lifetime must be a non-negative duration")
		}
	}
	for key, value := range result.InitialOptions {
		normalized, err := NormalizeOption(key, value)
		if err != nil {
			return Startup{}, err
		}
		result.InitialOptions[key] = normalized
	}
	return result, nil
}

func configPath(args []string, getenv func(string) string) (string, error) {
	path := getenv("STARHUB_CONFIG")
	for i := 0; i < len(args); i++ {
		switch {
		case args[i] == "--config":
			i++
			if i >= len(args) || args[i] == "" {
				return "", fmt.Errorf("--config requires a file path")
			}
			path = args[i]
		case strings.HasPrefix(args[i], "--config="):
			path = strings.TrimPrefix(args[i], "--config=")
			if path == "" {
				return "", fmt.Errorf("--config requires a file path")
			}
		default:
			return "", fmt.Errorf("unknown argument %q", args[i])
		}
	}
	return path, nil
}

// ApplyStartup sets defaults before the database seeds missing option keys.
func ApplyStartup(startup Startup) error {
	normalized := make(map[string]string, len(startup.InitialOptions))
	for key, value := range startup.InitialOptions {
		value, err := NormalizeOption(key, value)
		if err != nil {
			return err
		}
		normalized[key] = value
	}
	for key, value := range normalized {
		if err := ApplyOption(key, value); err != nil {
			return err
		}
	}
	forceSecureCookie.Store(startup.Bootstrap.ForceSecureCookie)
	return nil
}
