package model

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	mysqlDriver "github.com/go-sql-driver/mysql"
	"github.com/jackc/pgx/v5/pgconn"
	"gorm.io/gorm"
)

func TestProductionTransactionRetriesWholeRollbackOnly(t *testing.T) {
	newTestDB(t)
	attempts := 0
	err := productionTransaction(DB, func(tx *gorm.DB) error {
		attempts++
		if err := tx.Model(&ProductionStorageState{}).Where("id=1").UpdateColumn("dropped", gorm.Expr("dropped + 1")).Error; err != nil {
			return err
		}
		if attempts < 3 {
			return fmt.Errorf("wrapped: %w", &pgconn.PgError{Code: "40P01"})
		}
		return nil
	})
	state, readErr := ReadProductionStorage()
	if err != nil || readErr != nil || attempts != 3 || state.Dropped != 1 {
		t.Fatal("retry did not roll back the entire transaction", attempts, state, err, readErr)
	}
	for _, fault := range []error{&mysqlDriver.MySQLError{Number: 1406}, &pgconn.PgError{Code: "08006"}, ErrProductionEvidence} {
		attempts = 0
		if err := productionTransaction(DB, func(*gorm.DB) error { attempts++; return fault }); !errors.Is(err, fault) || attempts != 1 {
			t.Fatal("unsafe automatic retry", attempts, err)
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := productionTransaction(DB.WithContext(ctx), func(*gorm.DB) error { t.Fatal("canceled callback ran"); return nil }); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	if _, err := PruneProductionTelemetryContext(ctx, time.Now()); !errors.Is(err, context.Canceled) {
		t.Fatal("cleanup ignored cancellation", err)
	}
	if err := CompleteProductionDeliveryContext(ctx, nil, "", time.Now()); !errors.Is(err, context.Canceled) {
		t.Fatal("receipt ignored cancellation", err)
	}
}
