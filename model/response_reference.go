package model

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"math"
	"regexp"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"starhub/common"
)

const ResponseReferenceLifetime = 30 * 24 * time.Hour

var (
	ErrInvalidResponseReference   = errors.New("invalid stored response reference")
	ErrResponseReferenceNotFound  = errors.New("stored response reference not found")
	ErrResponseReferenceAmbiguous = errors.New("stored response reference has multiple upstream bindings")
	ErrResponseReferenceConflict  = errors.New("stored response reference metadata is immutable")
	responseReferenceID           = regexp.MustCompile(`^[A-Za-z0-9_-]{1,256}$`)
	responseReferenceDigest       = regexp.MustCompile(`^[a-f0-9]{64}$`)
)

// ResponseReference is a private ownership index, not a response-content store.
// Native dispatch captures this binding for scoped retrieval. Serialization
// must never expose credential fingerprints or internal ownership metadata.
type ResponseReference struct {
	ID                 int64  `gorm:"primaryKey;autoIncrement" json:"-"`
	UserID             int64  `gorm:"not null;uniqueIndex:idx_response_reference_binding,priority:1;index:idx_response_reference_lookup,priority:1" json:"-"`
	TokenID            int64  `gorm:"not null;uniqueIndex:idx_response_reference_binding,priority:2;index:idx_response_reference_lookup,priority:2" json:"-"`
	ResponseID         string `gorm:"size:256;not null;uniqueIndex:idx_response_reference_binding,priority:3;index:idx_response_reference_lookup,priority:3;index:idx_response_reference_account,priority:2" json:"-"`
	BindingID          string `gorm:"size:64;not null;uniqueIndex:idx_response_reference_binding,priority:4" json:"-"`
	ChannelID          int64  `gorm:"not null" json:"-"`
	CredentialIdentity string `gorm:"size:64;not null" json:"-"`
	AccountIdentity    string `gorm:"size:64;not null;default:'';index:idx_response_reference_account,priority:1" json:"-"`
	Conflicted         bool   `gorm:"not null;default:false" json:"-"`
	UpstreamModel      string `gorm:"size:128;not null;default:''" json:"-"`
	Model              string `gorm:"size:128;not null" json:"-"`
	Group              string `gorm:"size:64;not null" json:"-"`
	GroupDefinitionID  int64  `gorm:"not null;default:0" json:"-"`
	Status             string `gorm:"size:16;not null" json:"-"`
	TaskID             int64  `gorm:"not null;default:0;index" json:"-"`
	Background         bool   `gorm:"not null;default:false" json:"-"`
	DeletedAt          int64  `gorm:"not null;default:0" json:"-"`
	PromptTokens       int64  `gorm:"not null;default:0" json:"-"`
	CompletionTokens   int64  `gorm:"not null;default:0" json:"-"`
	HasImages          bool   `gorm:"not null;default:false" json:"-"`
	HasTools           bool   `gorm:"not null;default:false" json:"-"`
	HasReasoning       bool   `gorm:"not null;default:false" json:"-"`
	CreatedAt          int64  `gorm:"not null;autoCreateTime:false" json:"-"`
	ExpiresAt          int64  `gorm:"not null;index" json:"-"`
}

// ResponseCredentialIdentity binds the exact upstream account selected for a
// request. Pool order, unrelated keys, scheduling weights and traffic counters
// are deliberately excluded; an unrelated pool edit cannot rebind the account.
func ResponseCredentialIdentity(ch *Channel, selectedKey string) string {
	if ch == nil || ch.ID <= 0 || ch.Type <= 0 || ch.BaseURL == "" || selectedKey == "" {
		return ""
	}
	return responseBindingDigest([]any{ch.ID, ch.Type, []byte(ch.BaseURL), []byte(selectedKey)})
}

// ResponseAccountIdentity identifies the upstream storage namespace even when
// an administrator configures the same provider credential in two channels.
func ResponseAccountIdentity(ch *Channel, selectedKey string) string {
	if ch == nil || ch.Type <= 0 || ch.BaseURL == "" || selectedKey == "" {
		return ""
	}
	return responseBindingDigest([]any{ch.Type, []byte(strings.TrimRight(ch.BaseURL, "/")), []byte(selectedKey)})
}

// ResponseCredential resolves the original account without rotating the pool.
// A removed, disabled or replaced credential is never substituted with another.
func ResponseCredential(ch *Channel, identity string) (string, bool) {
	if ch == nil || ch.Status != common.ChannelStatusEnabled || !responseReferenceDigest.MatchString(identity) {
		return "", false
	}
	for index := range ch.Keys() {
		key, enabled := ch.KeyAt(index)
		if enabled && ResponseCredentialIdentity(ch, key) == identity {
			return key, true
		}
	}
	return "", false
}

func responseBindingDigest(parts []any) string {
	raw, _ := json.Marshal(parts)
	digest := sha256.Sum256(raw)
	return hex.EncodeToString(digest[:])
}

func validResponseReferenceLabel(value string, limit int) bool {
	if value == "" || len(value) > limit || !utf8.ValidString(value) || strings.TrimSpace(value) != value {
		return false
	}
	return !strings.ContainsFunc(value, unicode.IsControl)
}

func validResponseReference(ref ResponseReference, at time.Time) bool {
	if ref.UserID <= 0 || ref.TokenID <= 0 || ref.ChannelID <= 0 || ref.GroupDefinitionID < 0 ||
		!responseReferenceID.MatchString(ref.ResponseID) || !responseReferenceDigest.MatchString(ref.CredentialIdentity) || !responseReferenceDigest.MatchString(ref.AccountIdentity) ||
		!validResponseReferenceLabel(ref.Model, 128) || !validResponseReferenceLabel(ref.Group, 64) ||
		ref.PromptTokens < 0 || ref.CompletionTokens < 0 || ref.PromptTokens > math.MaxInt64-ref.CompletionTokens ||
		at.Unix() < 0 || at.Unix() > math.MaxInt64-int64(ResponseReferenceLifetime/time.Second) {
		return false
	}
	switch ref.Status {
	case "queued", "in_progress":
		return ref.Background && ref.TaskID > 0
	case "completed", "incomplete", "failed", "cancelled":
		return true
	}
	return false
}

// RecordResponseReference preserves the first observation and its deadline.
// Repeated observations cannot extend access or overwrite accounting/context.
func RecordResponseReference(ctx context.Context, ref ResponseReference, at time.Time) (*ResponseReference, error) {
	if !validResponseReference(ref, at) {
		return nil, ErrInvalidResponseReference
	}
	ref.ID = 0
	ref.Conflicted = false
	ref.BindingID = responseBindingDigest([]any{ref.ChannelID, ref.CredentialIdentity, ref.Model, ref.Group, ref.GroupDefinitionID})
	if ref.UpstreamModel != "" {
		ref.BindingID = responseBindingDigest([]any{ref.BindingID, ref.UpstreamModel})
	}
	ref.CreatedAt, ref.ExpiresAt = at.Unix(), at.Unix()+int64(ResponseReferenceLifetime/time.Second)
	var stored ResponseReference
	conflict := false
	err := DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&ref).Error; err != nil {
			return err
		}
		if err := tx.Where("user_id = ? AND token_id = ? AND response_id = ? AND binding_id = ?", ref.UserID, ref.TokenID, ref.ResponseID, ref.BindingID).First(&stored).Error; err != nil {
			return err
		}
		expected := ref
		expected.ID, expected.CreatedAt, expected.ExpiresAt = stored.ID, stored.CreatedAt, stored.ExpiresAt
		if stored.Conflicted || stored != expected {
			conflict = true
			return tx.Model(&ResponseReference{}).Where("id = ?", stored.ID).Update("conflicted", true).Error
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	if conflict {
		return nil, ErrResponseReferenceConflict
	}
	return &stored, nil
}

// LookupResponseReference never chooses between reused provider IDs. Expired
// collisions still count: a later binding must not revive an old public ID.
func LookupResponseReference(ctx context.Context, userID, tokenID int64, responseID string, at time.Time) (*ResponseReference, error) {
	if userID <= 0 || tokenID <= 0 || !responseReferenceID.MatchString(responseID) || at.Unix() < 0 {
		return nil, ErrResponseReferenceNotFound
	}
	var found []ResponseReference
	if err := DB.WithContext(ctx).Where("user_id = ? AND token_id = ? AND response_id = ?", userID, tokenID, responseID).Order("id").Limit(2).Find(&found).Error; err != nil {
		return nil, err
	}
	if len(found) > 1 {
		return nil, ErrResponseReferenceAmbiguous
	}
	if len(found) == 0 || found[0].DeletedAt != 0 || found[0].ExpiresAt <= at.Unix() || found[0].CreatedAt > at.Unix() {
		return nil, ErrResponseReferenceNotFound
	}
	ref := found[0]
	if ref.Conflicted || !responseReferenceDigest.MatchString(ref.AccountIdentity) {
		return nil, ErrResponseReferenceAmbiguous
	}
	var other int64
	if err := DB.WithContext(ctx).Model(&ResponseReference{}).
		Where("account_identity = ? AND response_id = ? AND (user_id <> ? OR token_id <> ? OR binding_id <> ?)", ref.AccountIdentity, responseID, userID, tokenID, ref.BindingID).
		Limit(1).Count(&other).Error; err != nil {
		return nil, err
	}
	if other != 0 {
		return nil, ErrResponseReferenceAmbiguous
	}
	return &found[0], nil
}

// ProviderModel preserves historical references without an alias binding.
func (ref *ResponseReference) ProviderModel() string {
	if ref.UpstreamModel != "" {
		return ref.UpstreamModel
	}
	return ref.Model
}
