package model

import (
	"errors"
	"time"

	"starhub/common"
)

// TokenRevealGrant binds repeated, audited viewing to one login and key.
// Only digests are persisted; key and account rotation revoke existing grants.
type TokenRevealGrant struct {
	Digest       string `gorm:"primaryKey;size:64"`
	UserID       int64  `gorm:"index;not null"`
	TokenID      int64  `gorm:"index;not null"`
	KeyHash      string `gorm:"size:64;not null"`
	PasswordHash string `gorm:"size:64;not null"`
	SessionHash  string `gorm:"size:64;not null"`
	ExpiresAt    int64  `gorm:"index;not null"`
}

func NewTokenRevealGrant(user *User, token *Token, session string) (string, int64, error) {
	secret := common.GenerateSessionKey()
	expires := time.Now().Add(5 * time.Minute).Unix()
	grant := &TokenRevealGrant{Digest: tokenKeyHash(secret), UserID: user.ID, TokenID: token.ID,
		KeyHash: token.KeyHash, PasswordHash: tokenKeyHash(user.Password), SessionHash: tokenKeyHash(session), ExpiresAt: expires}
	if err := DB.Where("expires_at <= ?", time.Now().Unix()).Delete(&TokenRevealGrant{}).Error; err != nil {
		return "", 0, err
	}
	err := DB.Create(grant).Error
	return secret, expires, err
}

func CheckTokenRevealGrant(secret, session string, user *User, token *Token) (int64, error) {
	var grant TokenRevealGrant
	err := DB.Where("digest = ? AND user_id = ? AND token_id = ? AND key_hash = ? AND password_hash = ? AND session_hash = ? AND expires_at > ?",
		tokenKeyHash(secret), user.ID, token.ID, token.KeyHash, tokenKeyHash(user.Password), tokenKeyHash(session), time.Now().Unix()).First(&grant).Error
	if err != nil {
		return 0, errors.New("key viewing authorization expired; verify your password again")
	}
	return grant.ExpiresAt, nil
}
