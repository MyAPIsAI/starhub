package model

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// RoutingChain pins a tool conversation within one token and service group.
// Only the identifier hash is stored; prompts and tool output are excluded.
type RoutingChain struct {
	TokenID   int64  `gorm:"primaryKey;autoIncrement:false"`
	Group     string `gorm:"primaryKey;size:64"`
	ChainHash string `gorm:"primaryKey;size:64"`
	Model     string `gorm:"size:128;not null"`
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
	return chain.Model, err
}

// PinRoutingChain returns the first winner under concurrent requests.
func PinRoutingChain(tokenID int64, group, id, actual string) (string, error) {
	hash := routingChainHash(id)
	chain := RoutingChain{TokenID: tokenID, Group: group, ChainHash: hash, Model: actual}
	if err := DB.Clauses(clause.OnConflict{DoNothing: true}).Create(&chain).Error; err != nil {
		return "", err
	}
	return FindRoutingChain(tokenID, group, id)
}
