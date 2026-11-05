package model

import (
	"context"
	"errors"
	"gorm.io/gorm"
	"starhub/common"
	"time"
)

type CustomerRequestLease struct {
	ID        string `gorm:"primaryKey;size:128"`
	UserID    int64  `gorm:"index"`
	ExpiresAt int64  `gorm:"index"`
}

var ErrCustomerConcurrency = errors.New("account concurrent request limit reached")

// A database lease enforces the account limit across gateway processes.
func AcquireCustomerRequest(ctx context.Context, id int64) (func(), error) {
	lease := CustomerRequestLease{ID: common.GenerateSessionKey(), UserID: id, ExpiresAt: time.Now().Add(5 * time.Minute).Unix()}
	err := DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&User{}).Where("id = ?", id).UpdateColumn("id", gorm.Expr("id")).Error; err != nil {
			return err
		}
		var user User
		if err := tx.First(&user, id).Error; err != nil {
			return err
		}
		if user.Status != common.StatusEnabled {
			return ErrUserNotFound
		}
		if err := tx.Where("expires_at <= ?", time.Now().Unix()).Delete(&CustomerRequestLease{}).Error; err != nil {
			return err
		}
		var active int64
		if err := tx.Model(&CustomerRequestLease{}).Where("user_id = ?", id).Count(&active).Error; err != nil {
			return err
		}
		limit := user.ConcurrencyLimit
		if limit <= 0 {
			limit = 50
		}
		if active >= int64(limit) {
			return ErrCustomerConcurrency
		}
		return tx.Create(&lease).Error
	})
	if err != nil {
		return nil, err
	}
	done := make(chan struct{})
	go func() {
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-done:
				return
			case <-ctx.Done():
				return
			case <-ticker.C:
				_ = DB.Model(&CustomerRequestLease{}).Where("id = ?", lease.ID).Update("expires_at", time.Now().Add(5*time.Minute).Unix()).Error
			}
		}
	}()
	return func() { close(done); _ = DB.Where("id = ?", lease.ID).Delete(&CustomerRequestLease{}).Error }, nil
}
