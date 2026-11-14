package model

import (
	"errors"
	"time"

	"github.com/glebarez/sqlite"
	mysqlDriver "github.com/go-sql-driver/mysql"
	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func openConfiguredDatabase(cfg DatabaseConfig) (*gorm.DB, error) {
	var dialect gorm.Dialector
	switch cfg.Driver {
	case "sqlite":
		dialect = sqlite.Open(dsn(cfg.DSN))
	case "mysql":
		connection, err := mysqlDriver.ParseDSN(cfg.DSN)
		if err != nil {
			return nil, errors.New("invalid MySQL DSN")
		}
		connection.ClientFoundRows = true
		connection.ParseTime = true
		connection.Loc = time.UTC
		connection.MaxAllowedPacket = 0
		if connection.Params == nil {
			connection.Params = map[string]string{}
		}
		// Existing static SQL quotes reserved identifiers with double quotes.
		mode := connection.Params["sql_mode"]
		if mode == "" {
			mode = "@@sql_mode"
		}
		connection.Params["sql_mode"] = "CONCAT(" + mode + ",',ANSI_QUOTES')"
		dialect = mysql.Open(connection.FormatDSN())
	case "postgres":
		dialect = postgres.Open(cfg.DSN)
	default:
		return nil, errors.New("unsupported database driver")
	}
	db, err := gorm.Open(dialect, &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		return nil, err
	}
	if cfg.Driver == "mysql" {
		var packet int64
		if err := db.Raw("SELECT @@max_allowed_packet").Scan(&packet).Error; err != nil || packet < 65<<20 {
			if pool, e := db.DB(); e == nil {
				_ = pool.Close()
			}
			return nil, errors.New("MySQL max_allowed_packet must be at least 65 MiB for telemetry manifests")
		}
		db = db.Set("gorm:table_options", "ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_bin").Session(&gorm.Session{})
	}
	return db, nil
}
