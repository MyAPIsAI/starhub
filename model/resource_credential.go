package model

import (
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"os"
	"time"

	"gorm.io/gorm"
)

// ResourceCredential stores only digests, never recoverable bearer tokens.
type ResourceCredential struct {
	ID            int `gorm:"primaryKey"`
	Current       string
	Previous      string
	PreviousUntil int64
	Disabled      bool
}

func resourceCredentialDigest(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

func ResourceCredentialValid(token string, now time.Time) bool {
	if token == "" || DB == nil {
		return false
	}
	var credential ResourceCredential
	err := DB.First(&credential, "id = 1").Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		initial := os.Getenv("METRIC_AUTHORIZATION")
		return initial != "" && subtle.ConstantTimeCompare([]byte(resourceCredentialDigest(token)), []byte(resourceCredentialDigest(initial))) == 1
	}
	if err != nil || credential.Disabled {
		return false
	}
	digest := resourceCredentialDigest(token)
	return subtle.ConstantTimeCompare([]byte(digest), []byte(credential.Current)) == 1 || (now.Unix() < credential.PreviousUntil && subtle.ConstantTimeCompare([]byte(digest), []byte(credential.Previous)) == 1)
}

// RotateResourceCredential starts one fixed ten-minute overlap. Repeating the
// current token is idempotent and cannot extend the previous token's lifetime.
func RotateResourceCredential(token string, disabled bool, now time.Time) error {
	if !disabled && len(token) < 32 {
		return errors.New("resource credential must contain at least 32 characters")
	}
	return DB.Transaction(func(tx *gorm.DB) error {
		initial := ResourceCredential{ID: 1}
		if value := os.Getenv("METRIC_AUTHORIZATION"); value != "" {
			initial.Current = resourceCredentialDigest(value)
		}
		if err := tx.Where("id = 1").FirstOrCreate(&initial).Error; err != nil {
			return err
		}
		if disabled {
			initial.Disabled, initial.Current, initial.Previous, initial.PreviousUntil = true, "", "", 0
		} else {
			digest := resourceCredentialDigest(token)
			if !initial.Disabled && initial.Current == digest {
				return nil
			}
			initial.Previous, initial.PreviousUntil = initial.Current, now.Add(10*time.Minute).Unix()
			initial.Current, initial.Disabled = digest, false
		}
		return tx.Save(&initial).Error
	})
}
