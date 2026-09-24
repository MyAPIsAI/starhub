package config

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"

	"gopkg.in/yaml.v3"
)

type Bootstrap struct {
	DBPath            string
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
		Port *int `yaml:"port"`
	} `yaml:"server"`
	Database struct {
		SQLitePath *string `yaml:"sqlite_path"`
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
		Bootstrap:      Bootstrap{DBPath: "starhub.db", Port: "3000", RootPassword: getenv("STARHUB_ROOT_PASSWORD")},
		InitialOptions: make(map[string]string),
	}
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
		if cfg.Server.Port != nil {
			result.Bootstrap.Port = strconv.Itoa(*cfg.Server.Port)
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
	if value := getenv("STARHUB_PORT"); value != "" {
		result.Bootstrap.Port = value
	}
	if value := getenv("STARHUB_DB"); value != "" {
		result.Bootstrap.DBPath = value
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
	port, err := strconv.Atoi(result.Bootstrap.Port)
	if err != nil || port < 1 || port > 65535 {
		return Startup{}, fmt.Errorf("server port must be in [1,65535]")
	}
	if strings.TrimSpace(result.Bootstrap.DBPath) == "" {
		return Startup{}, fmt.Errorf("database path must not be empty")
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
