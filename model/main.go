package model

import (
	"fmt"
	"strings"
	"time"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"starhub/common"
)

var DB *gorm.DB

// DatabaseConfig mirrors config.DatabaseConfig so InitDBWithConfig can
// be called without importing the config package.
type DatabaseConfig struct {
	Driver          string
	DSN             string
	MaxOpenConns    int
	MaxIdleConns    int
	ConnMaxLifetime string
}

// dsn applies the per-connection pragmas every pooled connection needs:
// busy_timeout makes concurrent billing writers queue instead of failing
// with SQLITE_BUSY, WAL keeps readers flowing while a writer commits.
func dsn(path string) string {
	separator := "?"
	if strings.Contains(path, "?") {
		separator = "&"
	}
	return path + separator + "_pragma=busy_timeout(10000)&_pragma=journal_mode(WAL)"
}

// InitDB opens a SQLite database at the given file path. It is kept for
// backward compatibility with tests and command-line tools. New callers
// should use InitDBWithConfig to select the driver.
func InitDB(path string) error {
	return InitDBWithConfig(DatabaseConfig{Driver: "sqlite", DSN: path})
}

// InitDBWithConfig keeps unported backends out of migrations and business
// transactions. SQLite retains its existing pragmas and pool behavior.
func InitDBWithConfig(cfg DatabaseConfig) (initErr error) {
	if cfg.Driver == "" {
		cfg.Driver = "sqlite"
	}
	switch cfg.Driver {
	case "sqlite":
	case "mysql", "postgres":
		return fmt.Errorf("%s support is not ready: migrations, telemetry triggers and transaction locking still require dialect adaptation", cfg.Driver)
	default:
		return fmt.Errorf("unsupported driver: %s", cfg.Driver)
	}
	if strings.TrimSpace(cfg.DSN) == "" {
		return fmt.Errorf("database DSN must not be empty")
	}
	db, err := gorm.Open(sqlite.Open(dsn(cfg.DSN)), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		return fmt.Errorf("open %s database: %w", cfg.Driver, err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		return err
	}
	previous := DB
	defer func() {
		if initErr != nil {
			if DB == db {
				DB = previous
			}
			_ = sqlDB.Close()
		}
	}()
	if err := db.AutoMigrate(&User{}, &Token{}, &Channel{}, &Ability{}, &Log{}, &Option{}, &OptionRevision{}, &OptionChange{}, &OptionInstance{}, &Task{}, &TaskPlugin{}, &RoutingChain{}, &QuotaSettlement{}, &AffiliateRebate{}, &AuditLog{}, &Redemption{}, &ModelMeta{}, &ModelMetadataState{}, &Wallet{}, &LedgerEntry{}, &RechargeOrder{}, &ExportRecord{}, &ServiceGroupConfig{}, &ServiceGroupDeletion{}, &ServiceQualityMetric{}, &ResourceCatalogRecord{}, &ResourcePlacementRecord{}, &ResourceRegion{}, &UserPhoneIdentity{}, &PhoneVerificationChallenge{}, &UserAgreement{}, &EmailVerificationChallenge{}, &TwoFA{}, &TwoFABackupCode{}, &RegionProfile{}, &LegalDocument{}, &ServiceIncident{}, &ServiceIncidentUpdate{}, &AvailabilityRecord{}, &ReconciliationRecord{}); err != nil {
		return fmt.Errorf("migrate schema: %w", err)
	}
	if err := db.AutoMigrate(&ChannelConfigurationState{}); err != nil {
		return fmt.Errorf("migrate channel configuration revision: %w", err)
	}
	if err := db.AutoMigrate(&TokenQuotaPeriod{}, &QuotaReservation{}); err != nil {
		return fmt.Errorf("migrate token quota periods: %w", err)
	}
	if err := db.AutoMigrate(&MoneyAccount{}, &MoneyEntry{}, &TokenUsageBaseline{}); err != nil {
		return fmt.Errorf("migrate exact monetary journal: %w", err)
	}
	if err := db.AutoMigrate(&ReconciliationRevision{}); err != nil {
		return err
	}
	if err := db.Exec("CREATE UNIQUE INDEX IF NOT EXISTS idx_reconciliation_report_key ON reconciliation_records(report_key) WHERE report_key <> ''").Error; err != nil {
		return err
	}
	if err := db.AutoMigrate(&ResponseReference{}); err != nil {
		return fmt.Errorf("migrate response ownership index: %w", err)
	}
	if err := db.AutoMigrate(&ResponseConversation{}); err != nil {
		return fmt.Errorf("migrate response conversations: %w", err)
	}
	if err := db.AutoMigrate(&UserSecurityEvent{}); err != nil {
		return fmt.Errorf("migrate user security history: %w", err)
	}
	if err := db.AutoMigrate(&AccountDeactivationReceipt{}); err != nil {
		return fmt.Errorf("migrate account deactivation receipts: %w", err)
	}
	if err := db.AutoMigrate(&OwnedTokenReceipt{}); err != nil {
		return fmt.Errorf("migrate owned token receipts: %w", err)
	}
	if err := db.AutoMigrate(&ResourceGatewayManagedChannel{}, &ResourceGatewayRoute{}, &ResourceGatewayPublication{}, &ResourceGatewayBindingChange{}); err != nil {
		return fmt.Errorf("migrate resource gateway ownership: %w", err)
	}
	if err := db.Exec("CREATE UNIQUE INDEX IF NOT EXISTS idx_resource_gateway_source_binding ON resource_gateway_managed_channels(source_channel_id) WHERE source_channel_id <> ''").Error; err != nil {
		return fmt.Errorf("migrate unique resource source bindings: %w", err)
	}
	if err := db.FirstOrCreate(&ChannelConfigurationState{}, ChannelConfigurationState{ID: 1}).Error; err != nil {
		return fmt.Errorf("initialize channel configuration revision: %w", err)
	}
	if err := db.FirstOrCreate(&OptionRevision{}, OptionRevision{ID: 1}).Error; err != nil {
		return fmt.Errorf("initialize option revision: %w", err)
	}
	if err := db.FirstOrCreate(&ResourceCatalogRecord{}, ResourceCatalogRecord{ID: 1}).Error; err != nil {
		return fmt.Errorf("initialize resource catalog: %w", err)
	}
	if err := db.FirstOrCreate(&ModelMetadataState{}, ModelMetadataState{ID: 1}).Error; err != nil {
		return fmt.Errorf("initialize model metadata catalog: %w", err)
	}
	// Give pre-lifecycle pins one full idle period during schema migration.
	if err := db.Model(&RoutingChain{}).Where("expires_at = 0").UpdateColumn("expires_at", time.Now().Add(RoutingChainTTL).Unix()).Error; err != nil {
		return fmt.Errorf("initialize routing chain expiry: %w", err)
	}
	if err := db.AutoMigrate(&RoutingSnapshotRecord{}, &ResourceCredential{}, &PlatformCatalogRecord{}, &RoutingGroupPolicy{}, &LocalRoutingState{}, &ResourceEventOutbox{}, &ResourceObservationWork{}); err != nil {
		return fmt.Errorf("migrate resource platform state: %w", err)
	}
	if err := db.AutoMigrate(&ResourceFailureStream{}); err != nil {
		return fmt.Errorf("migrate resource failure stream: %w", err)
	}
	if err := initRoutingWriter(db); err != nil {
		return fmt.Errorf("initialize routing writer: %w", err)
	}
	if err := db.AutoMigrate(&ProductionTelemetryMinute{}, &ProductionAttemptRecord{}); err != nil {
		return fmt.Errorf("migrate production telemetry: %w", err)
	}
	if err := initProductionStorage(db); err != nil {
		return fmt.Errorf("initialize production storage budget: %w", err)
	}
	DB = db
	if _, err := MigrateCredentialStorage(false, "system", "Upgrade legacy credential storage"); err != nil {
		return fmt.Errorf("migrate credential storage: %w", err)
	}
	ResetRoutingSnapshot()
	ResetRoutingFailureStream()
	return nil
}

func CloseDB() error {
	if DB == nil {
		return nil
	}
	sqlDB, err := DB.DB()
	if err != nil {
		return err
	}
	DB = nil
	return sqlDB.Close()
}

func Seed(rootPassword string) error {
	var count int64
	if err := DB.Model(&User{}).Count(&count).Error; err != nil {
		return err
	}
	if count == 0 {
		hashed, err := common.HashPassword(rootPassword)
		if err != nil {
			return err
		}
		root := &User{
			Username: common.DefaultRootUser,
			Password: hashed,
			Role:     common.RoleRoot,
			Status:   common.StatusEnabled,
			Quota:    100000000,
			Group:    common.DefaultGroup,
			AffCode:  common.GenerateAffCode(),
		}
		if err := DB.Create(root).Error; err != nil {
			return err
		}
	}
	return SeedOptions()
}
