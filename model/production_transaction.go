package model

import (
	"context"
	"database/sql/driver"
	"errors"
	"math/rand/v2"
	"net"
	"os"
	"strings"
	"sync/atomic"
	"time"

	mysqlDriver "github.com/go-sql-driver/mysql"
	"github.com/jackc/pgx/v5/pgconn"
	"gorm.io/gorm"

	"starhub/metric"
)

var productionTransactionRetries atomic.Int64
var productionDatabaseTimings metric.DurationHistogram

func ProductionDatabaseTimeout() time.Duration {
	if value, err := time.ParseDuration(os.Getenv("STARHUB_TELEMETRY_DB_TIMEOUT")); err == nil && value > 0 {
		return value
	}
	return 5 * time.Second
}

// DatabaseErrorKind separates data failures from faults a whole transaction
// can safely retry. Connection failures at commit have an unknown outcome.
func DatabaseErrorKind(err error) string {
	if err == nil {
		return ""
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return "timeout"
	}
	if strings.Contains(err.Error(), "production storage capacity exceeded") {
		return "capacity"
	}
	var my *mysqlDriver.MySQLError
	if errors.As(err, &my) {
		switch my.Number {
		case 1213:
			return "deadlock"
		case 1205:
			return "lock_timeout"
		case 1040, 2006, 2013:
			return "connection"
		case 1048, 1062, 1264, 1366, 1406:
			return "data"
		}
	}
	var pg *pgconn.PgError
	if errors.As(err, &pg) {
		switch pg.Code {
		case "40P01", "40001":
			return "deadlock"
		case "55P03":
			return "lock_timeout"
		}
		if strings.HasPrefix(pg.Code, "08") {
			return "connection"
		}
		if strings.HasPrefix(pg.Code, "22") || strings.HasPrefix(pg.Code, "23") {
			return "data"
		}
	}
	var network net.Error
	if errors.As(err, &network) || errors.Is(err, driver.ErrBadConn) {
		return "connection"
	}
	if errors.Is(err, ErrProductionEvidence) {
		return "data"
	}
	return "other"
}

var ErrProductionEvidence = errors.New("invalid production evidence")

func productionTransaction(db *gorm.DB, fn func(*gorm.DB) error) error {
	ctx, cancel := context.WithTimeout(db.Statement.Context, ProductionDatabaseTimeout())
	defer cancel()
	db = db.WithContext(ctx)
	started := time.Now()
	defer func() { productionDatabaseTimings.Observe(time.Since(started)) }()
	for attempt := 0; ; attempt++ {
		err := db.Transaction(fn)
		kind := DatabaseErrorKind(err)
		if err == nil || attempt >= 2 || (kind != "deadlock" && kind != "lock_timeout") || ctx.Err() != nil {
			return err
		}
		productionTransactionRetries.Add(1)
		timer := time.NewTimer(time.Duration(10*(attempt+1)+rand.IntN(10)) * time.Millisecond)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
	}
}

func ProductionDatabaseRetries() int64 { return productionTransactionRetries.Load() }

func ProductionDatabaseTimings() metric.DurationSnapshot { return productionDatabaseTimings.Snapshot() }
