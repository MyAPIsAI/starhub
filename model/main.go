package model

import (
	"fmt"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"starhub/common"
)

var DB *gorm.DB

func InitDB(path string) error {
	db, err := gorm.Open(sqlite.Open(path), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		return fmt.Errorf("open sqlite database %s: %w", path, err)
	}
	if err := db.AutoMigrate(&User{}, &Token{}, &Channel{}, &Ability{}, &Log{}, &Option{}); err != nil {
		return fmt.Errorf("migrate schema: %w", err)
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
		}
		if err := DB.Create(root).Error; err != nil {
			return err
		}
	}
	return SeedOptions()
}
