package model

import (
	"encoding/json"
	"errors"
	"gorm.io/gorm"
	"starhub/common"
	"time"
)

var ErrConversationUnavailable = errors.New("conversation unavailable or busy")

type ResponseConversation struct {
	ID             string `gorm:"primaryKey;size:80" json:"id"`
	UserID         int64  `gorm:"index;not null" json:"-"`
	TokenID        int64  `gorm:"index;not null" json:"-"`
	Metadata       string `gorm:"type:text;not null" json:"-"`
	LastResponseID string `gorm:"size:256;not null;default:''" json:"-"`
	Version        int64  `gorm:"not null;default:0" json:"-"`
	Busy           bool   `gorm:"not null;default:false" json:"blocked"`
	Nonce          string `gorm:"size:64;not null;default:''" json:"-"`
	DeletedAt      int64  `gorm:"not null;default:0" json:"-"`
	CreatedAt      int64  `gorm:"autoCreateTime" json:"created_at"`
	ExpiresAt      int64  `gorm:"not null" json:"-"`
}
type ResponseConversationLease struct {
	Conversation                 *ResponseConversation
	Nonce                        string
	Claimed, Sent, Handoff, Done bool
}

func CreateResponseConversation(owner, token int64, metadata map[string]string) (*ResponseConversation, error) {
	var result ResponseConversation
	err := DB.Transaction(func(tx *gorm.DB) error {
		lock := tx.Model(&User{}).Where("id = ? AND status = 1", owner).UpdateColumn("id", gorm.Expr("id"))
		if lock.Error != nil {
			return lock.Error
		}
		if lock.RowsAffected != 1 {
			return ErrConversationUnavailable
		}
		var count int64
		if err := tx.Model(&ResponseConversation{}).Where("user_id = ? AND token_id = ? AND deleted_at = 0 AND expires_at > ?", owner, token, time.Now().Unix()).Count(&count).Error; err != nil {
			return err
		}
		if count >= 128 {
			return ErrConversationUnavailable
		}
		raw, _ := json.Marshal(metadata)
		result = ResponseConversation{ID: "conv_sh_" + common.GenerateSessionKey(), UserID: owner, TokenID: token, Metadata: string(raw), ExpiresAt: time.Now().Add(ResponseReferenceLifetime).Unix()}
		return tx.Create(&result).Error
	})
	return &result, err
}
func GetResponseConversation(id string, owner, token int64) (*ResponseConversation, error) {
	var result ResponseConversation
	err := DB.Where("id = ? AND user_id = ? AND token_id = ? AND deleted_at = 0 AND expires_at > ?", id, owner, token, time.Now().Unix()).First(&result).Error
	return &result, err
}
func ClaimResponseConversation(conversation *ResponseConversation, nonce string) error {
	res := DB.Model(&ResponseConversation{}).Where("id = ? AND user_id = ? AND token_id = ? AND version = ? AND busy = ? AND deleted_at = 0 AND expires_at > ?", conversation.ID, conversation.UserID, conversation.TokenID, conversation.Version, false, time.Now().Unix()).Updates(map[string]any{"busy": true, "nonce": nonce, "version": gorm.Expr("version + 1")})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected != 1 {
		return ErrConversationUnavailable
	}
	return nil
}
func finishResponseConversation(tx *gorm.DB, id, nonce, responseID string) error {
	if id == "" {
		return nil
	}
	fields := map[string]any{"busy": false, "nonce": ""}
	if responseID != "" {
		fields["last_response_id"] = responseID
	}
	return tx.Model(&ResponseConversation{}).Where("id = ? AND nonce = ? AND busy = ? AND deleted_at = 0", id, nonce, true).Updates(fields).Error
}
func FinishResponseConversation(id, nonce, responseID string) error {
	return finishResponseConversation(DB, id, nonce, responseID)
}
func DeleteResponseConversation(conversation *ResponseConversation) error {
	return DB.Model(&ResponseConversation{}).Where("id = ? AND user_id = ? AND token_id = ?", conversation.ID, conversation.UserID, conversation.TokenID).Update("deleted_at", time.Now().Unix()).Error
}
