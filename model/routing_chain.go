package model

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"time"

	"gorm.io/gorm"
)

var ErrRoutingChainConflict = errors.New("routing chain was concurrently pinned to another model; retry the request")
var ErrRoutingChainExpired = errors.New("routing chain is missing or expired; start a new conversation with a new chain identifier")
var ErrRoutingChainLimit = errors.New("active routing chain limit reached for this token; reuse an existing chain or wait for expiry")

const RoutingChainTTL = 24 * time.Hour
const MaxRoutingChainsPerToken = 1024

// RoutingChain pins a tool conversation within one token and service group.
// Only the identifier hash is stored; prompts and tool output are excluded.
type RoutingChain struct {
	TokenID   int64  `gorm:"primaryKey;autoIncrement:false"`
	Group     string `gorm:"primaryKey;size:64"`
	ChainHash string `gorm:"primaryKey;size:64"`
	Model     string `gorm:"size:128;not null"`
	ExpiresAt int64  `gorm:"index;not null;default:0"`
}

func routingChainHash(id string) string {
	hash := sha256.Sum256([]byte(id))
	return hex.EncodeToString(hash[:])
}
func FindRoutingChain(tokenID int64, group, id string) (string, error) {
	var chain RoutingChain
	err := DB.Where("token_id = ? AND \"group\" = ? AND chain_hash = ?", tokenID, group, routingChainHash(id)).First(&chain).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return "", nil
	}
	if err == nil && chain.ExpiresAt > 0 && chain.ExpiresAt <= time.Now().Unix() {
		return "", ErrRoutingChainExpired
	}
	return chain.Model, err
}

// PinRoutingChain returns the first winner under concurrent requests.
func PinRoutingChain(tokenID int64, group, id, actual string) (string, error) {
	return admitRoutingChain(tokenID, group, id, actual, false, time.Now().Unix())
}

// AdmitRoutingChain also requires an existing, unexpired pin for tool results.
// The transaction checks expiry again after quota admission to close races.
func AdmitRoutingChain(tokenID int64, group, id, actual string, continuation bool) (string, error) {
	return admitRoutingChain(tokenID, group, id, actual, continuation, time.Now().Unix())
}

func admitRoutingChain(tokenID int64, group, id, actual string, continuation bool, now int64) (string, error) {
	var winner string
	err := DB.Transaction(func(tx *gorm.DB) error {
		// Acquire SQLite's writer lock before reads so concurrent admissions
		// cannot both observe capacity or an absent pin and then insert.
		if err := tx.Model(&RoutingChain{}).Where("token_id = ? AND \"group\" = ? AND chain_hash = ?", tokenID, group, routingChainHash(id)).UpdateColumn("expires_at", gorm.Expr("expires_at")).Error; err != nil {
			return err
		}
		var chain RoutingChain
		query := tx.Where("token_id = ? AND \"group\" = ? AND chain_hash = ?", tokenID, group, routingChainHash(id))
		err := query.First(&chain).Error
		if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		exists := err == nil
		if exists && chain.ExpiresAt > 0 && chain.ExpiresAt <= now {
			return ErrRoutingChainExpired
		}
		if !exists && continuation {
			return ErrRoutingChainExpired
		}
		if exists {
			winner = chain.Model
			if winner != actual {
				return nil
			}
			return tx.Model(&chain).UpdateColumn("expires_at", now+int64(RoutingChainTTL/time.Second)).Error
		}
		if err := tx.Where("token_id = ? AND expires_at > 0 AND expires_at <= ?", tokenID, now).Delete(&RoutingChain{}).Error; err != nil {
			return err
		}
		var count int64
		if err := tx.Model(&RoutingChain{}).Where("token_id = ?", tokenID).Count(&count).Error; err != nil {
			return err
		}
		if count >= MaxRoutingChainsPerToken {
			return ErrRoutingChainLimit
		}
		chain = RoutingChain{TokenID: tokenID, Group: group, ChainHash: routingChainHash(id), Model: actual, ExpiresAt: now + int64(RoutingChainTTL/time.Second)}
		if err := tx.Create(&chain).Error; err != nil {
			return err
		}
		winner = actual
		return nil
	})
	return winner, err
}

// CleanupExpiredRoutingChains never removes active or legacy unexpired pins.
func CleanupExpiredRoutingChains(now int64) error {
	return DB.Where("expires_at > 0 AND expires_at <= ?", now).Delete(&RoutingChain{}).Error
}
