package model

import (
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// RoutingWriteGuard preserves one lock order for snapshot and circuit writes,
// including the first creation of a resource's local state.
type RoutingWriteGuard struct {
	ID int `gorm:"primaryKey;autoIncrement:false"`
}

func initRoutingWriter(db *gorm.DB) error {
	if db.Dialector.Name() == "sqlite" {
		return nil
	}
	if err := db.AutoMigrate(&RoutingWriteGuard{}); err != nil {
		return err
	}
	return db.Clauses(clause.OnConflict{DoNothing: true}).Create(&RoutingWriteGuard{ID: 1}).Error
}

func lockRoutingWriter(tx *gorm.DB) error {
	if tx.Dialector.Name() == "sqlite" {
		return tx.Exec("UPDATE local_routing_states SET resource_key=resource_key WHERE resource_key = ''").Error
	}
	if err := tx.Model(&RoutingWriteGuard{}).Where("id=1").UpdateColumn("id", gorm.Expr("id")).Error; err != nil {
		return err
	}
	var guard RoutingWriteGuard
	return tx.First(&guard, 1).Error
}
