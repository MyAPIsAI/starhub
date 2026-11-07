package model

import (
	"fmt"
	"time"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"starhub/common"
)

var DB *gorm.DB

// dsn applies the per-connection pragmas every pooled connection needs:
// busy_timeout makes concurrent billing writers queue instead of failing
// with SQLITE_BUSY, WAL keeps readers flowing while a writer commits.
func dsn(path string) string {
	return path + "?_pragma=busy_timeout(10000)&_pragma=journal_mode(WAL)"
}

func InitDB(path string) error {
	aead, err := openTokenCipher(path)
	if err != nil {
		return err
	}
	db, err := gorm.Open(sqlite.Open(dsn(path)), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		return fmt.Errorf("open sqlite database %s: %w", path, err)
	}
	initialized := false
	defer func() {
		if !initialized {
			if connection, err := db.DB(); err == nil {
				_ = connection.Close()
			}
		}
	}()
	db = db.Set(tokenCipherSetting, aead).Session(&gorm.Session{})
	legacyReservations := !db.Migrator().HasColumn(&User{}, "reserved_quota")
	legacyQuotaLimits := !db.Migrator().HasColumn(&Token{}, "quota_limit")
	legacyStaffAccounts := !db.Migrator().HasColumn(&User{}, "account_kind")
	if err := db.AutoMigrate(&User{}, &Token{}, &Channel{}, &Ability{}, &Log{}, &Option{}, &OptionRevision{}, &OptionChange{}, &OptionInstance{}, &Task{}, &TaskPlugin{}, &RoutingChain{}, &QuotaSettlement{}, &AffiliateRebate{}, &AuditLog{}, &Redemption{}, &ModelMeta{}, &ModelMetadataState{}, &Wallet{}, &LedgerEntry{}, &RechargeOrder{}, &ExportRecord{}, &ServiceGroupConfig{}, &ServiceGroupDeletion{}, &ServiceQualityMetric{}, &ResourceCatalogRecord{}, &ResourcePlacementRecord{}, &ResourcePlacementArchive{}, &ResourceRegion{}); err != nil {
		return fmt.Errorf("migrate schema: %w", err)
	}
	if err := db.AutoMigrate(&ChannelConfigurationState{}, &TransactionalMail{}); err != nil {
		return fmt.Errorf("migrate channel configuration revision: %w", err)
	}
	if err := db.AutoMigrate(&ResourceSyncState{}, &ResourceSyncFailure{}, &GatewayResourceReport{}); err != nil {
		return fmt.Errorf("migrate resource monitoring: %w", err)
	}
	if err := db.AutoMigrate(&ResourceMonitorFeed{}, &ResourceDirectoryPrice{}, &ResourceObservedEvent{}, &ResourcePriceMismatch{}, &ResourceIncident{}); err != nil {
		return fmt.Errorf("migrate resource monitoring history: %w", err)
	}
	if err := db.AutoMigrate(&ServiceAnnouncement{}); err != nil {
		return err
	}
	if err := db.AutoMigrate(&DeploymentProfileState{}, &DeploymentProfileVersion{}, &LegalDocumentVersion{}, &StandardModel{}, &StandardModelHistory{}, &CustomerRequestLease{}, &ReconciliationReport{}, &ReconciliationDirtyDay{}, &ReconciliationSchedule{}, &ReconciliationAlert{}, &PaymentStatement{}, &PaymentStatementRow{}); err != nil {
		return err
	}
	if err := db.FirstOrCreate(&DeploymentProfileState{}, DeploymentProfileState{ID: 1}).Error; err != nil {
		return err
	}
	if legacyStaffAccounts {
		if err := db.Model(&User{}).Where("role IN ?", []int{common.RoleAdmin, common.RoleFinance, common.RoleTechnical, common.RoleRoot}).Updates(map[string]any{"account_kind": "staff", "must_change_password": true, "security_version": gorm.Expr("security_version + 1")}).Error; err != nil {
			return err
		}
	}
	if err := db.AutoMigrate(&StaffAccount{}); err != nil {
		return fmt.Errorf("migrate staff schema: %w", err)
	}
	if err := migrateStaffIdentities(db); err != nil {
		return fmt.Errorf("migrate staff identities: %w", err)
	}
	if err := db.AutoMigrate(&APIChargeEvent{}); err != nil {
		return fmt.Errorf("migrate API billing outbox: %w", err)
	}
	if legacyReservations {
		if err := db.Exec("UPDATE users SET reserved_quota = COALESCE((SELECT SUM(quota) FROM tasks WHERE tasks.user_id = users.id AND status NOT IN ('succeeded','failed','cancelled')),0) + COALESCE((SELECT SUM(actual-delta) FROM quota_settlements WHERE quota_settlements.user_id = users.id AND completed_at = 0),0)").Error; err != nil {
			return err
		}
	}
	if err := initializeExportMetadata(db); err != nil {
		return fmt.Errorf("migrate export download metadata: %w", err)
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
	if err := db.AutoMigrate(&TokenRevealGrant{}); err != nil {
		return fmt.Errorf("migrate key viewing authorizations: %w", err)
	}
	if err := db.AutoMigrate(&TokenQuotaPeriod{}); err != nil {
		return fmt.Errorf("migrate key quota periods: %w", err)
	}
	if legacyQuotaLimits {
		if err := db.Exec("UPDATE tokens SET quota_limit = remain_quota + used_quota + COALESCE((SELECT SUM(quota) FROM tasks WHERE tasks.token_id = tokens.id AND token_unlimited = 0 AND status NOT IN ('succeeded', 'failed', 'cancelled')), 0)").Error; err != nil {
			return fmt.Errorf("migrate key quota limits: %w", err)
		}
	}
	if err := db.Exec("UPDATE tokens SET period_started_at = CASE WHEN created_at > 0 THEN created_at ELSE strftime('%s','now') END WHERE period_started_at = 0 AND quota_period = 'never'").Error; err != nil {
		return err
	}
	if err := db.AutoMigrate(&ResourceGatewayManagedChannel{}, &ResourceGatewayRoute{}, &ResourceGatewayPublication{}, &ResourceGatewayBindingChange{}); err != nil {
		return fmt.Errorf("migrate resource gateway ownership: %w", err)
	}
	if err := db.Exec(`CREATE UNIQUE INDEX IF NOT EXISTS idx_resource_gateway_source_binding ON resource_gateway_managed_channels (source_channel_id) WHERE source_channel_id <> ''`).Error; err != nil {
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
	if err := migrateTokenSecrets(db); err != nil {
		return fmt.Errorf("migrate key encryption: %w", err)
	}
	DB = db
	LocalResources = NewResourceRuntime(common.GenerateSessionKey())
	initialized = true
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
	if err := DB.Transaction(func(tx *gorm.DB) error { return seedRootAndGroups(tx, rootPassword) }); err != nil {
		return err
	}
	return SeedOptions()
}

func seedRootAndGroups(tx *gorm.DB, rootPassword string) error {
	var count int64
	if err := tx.Model(&User{}).Count(&count).Error; err != nil {
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
		if err := createStaffIdentity(tx, root); err != nil {
			return err
		}
		return seedDefaultServiceGroups(tx)
	}
	return nil
}
