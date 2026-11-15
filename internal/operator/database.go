package operator

import (
	"errors"
	"os"
	"strings"

	"starhub/common"
	"starhub/config"
	"starhub/model"
)

// OpenDatabase uses the server's configuration without creating a new SQLite
// file by accident. Network credentials are read from the environment or YAML.
func OpenDatabase(path string) error {
	if err := common.ValidateCredentialConfiguration(); err != nil {
		return err
	}
	startup, err := config.LoadStartup(nil, os.Getenv)
	if err != nil {
		return errors.New("invalid database configuration")
	}
	cfg := startup.Bootstrap.Database
	if cfg.Driver == "sqlite" {
		if path != "" {
			cfg.DSN = path
		}
		if strings.HasPrefix(cfg.DSN, "file:") || cfg.DSN == ":memory:" {
			return errors.New("operator commands require an existing SQLite file")
		}
		info, err := os.Stat(cfg.DSN)
		if err != nil || !info.Mode().IsRegular() {
			return errors.New("existing database required")
		}
	} else if path != "" {
		return errors.New("db path is only supported for SQLite; use database configuration for network drivers")
	}
	if err := model.InitDBWithConfig(model.DatabaseConfig{
		Driver: cfg.Driver, DSN: cfg.DSN, MaxOpenConns: cfg.MaxOpenConns,
		MaxIdleConns: cfg.MaxIdleConns, ConnMaxLifetime: cfg.ConnMaxLifetime,
	}); err != nil {
		return errors.New("database verification failed")
	}
	return nil
}
