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
	db, err := gorm.Open(sqlite.Open(dsn(path)), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		return fmt.Errorf("open sqlite database %s: %w", path, err)
	}
	if err := db.AutoMigrate(&User{}, &Token{}, &Channel{}, &Ability{}, &Log{}, &Option{}, &OptionRevision{}, &OptionChange{}, &OptionInstance{}, &Task{}, &RoutingChain{}, &QuotaSettlement{}, &AffiliateRebate{}, &AuditLog{}, &Redemption{}, &ModelMeta{}, &ModelMetadataState{}, &Wallet{}, &LedgerEntry{}, &RechargeOrder{}, &ExportRecord{}, &ServiceGroupConfig{}, &ServiceGroupDeletion{}, &ServiceQualityMetric{}, &ResourceCatalogRecord{}, &ResourcePlacementRecord{}, &ResourceRegion{}); err != nil {
		return fmt.Errorf("migrate schema: %w", err)
	}
	if err := db.AutoMigrate(&ChannelConfigurationState{}); err != nil {
		return fmt.Errorf("migrate channel configuration revision: %w", err)
	}
	if err := db.AutoMigrate(&ResponseReference{}); err != nil {
		return fmt.Errorf("migrate response ownership index: %w", err)
	}
	if err := db.AutoMigrate(&UserSecurityEvent{}); err != nil {
		return fmt.Errorf("migrate user security history: %w", err)
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
	DB = db
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
