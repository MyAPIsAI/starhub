package model

import (
	"errors"
	"net"
	"strings"

	"gorm.io/gorm"

	"starhub/common"
)

var ErrTokenNotFound = errors.New("token not found")
var ErrTokenLimitExceeded = errors.New("account has reached the maximum number of API keys (200)")
var ErrTokenKeyGeneration = errors.New("failed to generate a unique key after 3 attempts")

// AllowsSmartRouting describes existing routing semantics, not an extra grant.
// Virtual-only allowlists cannot authorize any actual upstream model.
// SmartAccess, when explicitly set to false, disables smart routing for
// this token regardless of model authorization.
func (t *Token) AllowsSmartRouting() bool {
	if t.SmartAccess != nil && !*t.SmartAccess {
		return false
	}
	if t.ModelLimits == "" {
		return true
	}
	for _, name := range t.ModelLimitList() {
		if name != "myapis-auto" && name != "veyra-auto" {
			return true
		}
	}
	return false
}

type Token struct {
	ID             int64  `gorm:"primaryKey;autoIncrement" json:"id"`
	UserID         int64  `gorm:"index;not null" json:"user_id"`
	Name           string `gorm:"size:64;not null;default:''" json:"name"`
	Key            string `gorm:"uniqueIndex;size:256;not null" json:"-"`           // encrypted ciphertext or legacy plaintext
	KeyHash        string `gorm:"uniqueIndex;size:64;not null;default:''" json:"-"` // HMAC-SHA256 for lookup
	Status         int    `gorm:"not null;default:1" json:"status"`
	Group          string `gorm:"column:group;size:64;not null;default:''" json:"group"`
	ExpiredTime    int64  `gorm:"not null;default:-1" json:"expired_time"`
	RemainQuota    int64  `gorm:"not null;default:0" json:"remain_quota"`
	UnlimitedQuota bool   `gorm:"not null;default:false" json:"unlimited_quota"`
	UsedQuota      int64  `gorm:"not null;default:0" json:"used_quota"`
	// ModelLimits is a comma-separated model whitelist; empty means no limit.
	// A sub-key can be restricted to cheaper models only.
	ModelLimits string `gorm:"size:1024;not null;default:''" json:"model_limits"`
	// AllowIps is a comma-separated IP whitelist; empty means no limit.
	// Enterprise customers use it to prevent key leakage.
	AllowIps string `gorm:"size:1024;not null;default:''" json:"allow_ips"`
	// AutoGroups is a comma-separated group sequence tried in order when the
	// token's group is "auto" (cheapest-first, degrade to pricier on failure).
	AutoGroups string `gorm:"size:512;not null;default:''" json:"auto_groups"`
	// SmartAccess is an independent flag controlling whether the token may
	// use the smart routing entry (myapis-auto/veyra-auto). When false, the
	// token can only call explicitly authorized models even if the group
	// supports smart routing. When true (the default for backward compat),
	// smart routing is allowed if the token also has model authorization.
	SmartAccess *bool `gorm:"default:null" json:"smart_access,omitempty"`
	CreatedAt   int64 `gorm:"autoCreateTime" json:"created_at"`
	Revision    int64 `gorm:"not null;default:0" json:"management_version"`
}

// MaxTokensPerUser is the maximum number of API keys a single account may own.
const MaxTokensPerUser = 200

// MaxAllowIpsPerToken is the maximum number of IP whitelist entries per key.
const MaxAllowIpsPerToken = 50

// ValidateAllowIpsCount checks that the comma-separated IP whitelist does not
// exceed MaxAllowIpsPerToken entries. Empty is allowed (no restriction).
var ErrTooManyAllowIps = errors.New("IP whitelist exceeds the maximum of 50 entries per key")

func ValidateAllowIpsCount(allowIps string) error {
	if allowIps == "" {
		return nil
	}
	if len(SplitList(allowIps)) > MaxAllowIpsPerToken {
		return ErrTooManyAllowIps
	}
	return nil
}

func InsertToken(t *Token) error {
	// Check per-account key limit inside a transaction to avoid TOCTOU race.
	return DB.Transaction(func(tx *gorm.DB) error {
		var count int64
		if err := tx.Model(&Token{}).Where("user_id = ?", t.UserID).Count(&count).Error; err != nil {
			return err
		}
		if count >= MaxTokensPerUser {
			return ErrTokenLimitExceeded
		}
		for i := 0; i < 3; i++ {
			plaintext := common.GenerateTokenKey()
			t.KeyHash = common.HashKey(plaintext)
			encrypted, encErr := common.Encrypt(plaintext)
			if encErr != nil {
				return encErr
			}
			t.Key = encrypted
			err := tx.Create(t).Error
			if err == nil {
				return nil
			}
			if !strings.Contains(err.Error(), "UNIQUE constraint failed") {
				return err
			}
		}
		return errors.New("failed to generate a unique key after 3 attempts")
	})
}

func GetTokenByKey(key string) (*Token, error) {
	if key == "" {
		return nil, ErrTokenNotFound
	}
	hash := common.HashKey(key)
	var t Token
	// Try hash-based lookup first (encrypted keys).
	err := DB.Where("key_hash = ?", hash).First(&t).Error
	if err == nil {
		return &t, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}
	// Fallback: plaintext lookup (legacy keys not yet migrated).
	err = DB.Where("key = ?", key).First(&t).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrTokenNotFound
	}
	if err != nil {
		return nil, err
	}
	return &t, nil
}

func GetTokenByID(id int64) (*Token, error) {
	if id <= 0 {
		return nil, ErrTokenNotFound
	}
	var t Token
	err := DB.First(&t, "id = ?", id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrTokenNotFound
	}
	if err != nil {
		return nil, err
	}
	return &t, nil
}

// PlaintextKey decrypts the stored Key and returns the plaintext. For
// legacy plaintext keys (not yet migrated), the value is returned as-is.
func (t *Token) PlaintextKey() string {
	plaintext, err := common.Decrypt(t.Key)
	if err != nil {
		// Decryption failed — return the raw stored value (best effort).
		return t.Key
	}
	return plaintext
}

func ListTokens(userID int64) ([]Token, error) {
	var tokens []Token
	q := DB.Model(&Token{})
	if userID > 0 {
		q = q.Where("user_id = ?", userID)
	}
	err := q.Order("id DESC").Find(&tokens).Error
	return tokens, err
}

func UpdateTokenFields(id int64, fields map[string]any) error {
	if len(fields) == 0 {
		return nil
	}
	fields["revision"] = gorm.Expr("revision + 1")
	return DB.Model(&Token{}).Where("id = ?", id).Updates(fields).Error
}

func DeleteToken(id int64) error {
	return DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Delete(&Token{}, "id = ?", id).Error; err != nil {
			return err
		}
		return tx.Where("token_id = ?", id).Delete(&RoutingChain{}).Error
	})
}

func DecreaseTokenQuota(id int64, amount int64) (bool, error) {
	if amount <= 0 {
		return true, nil
	}
	res := DB.Model(&Token{}).Where("id = ? AND remain_quota >= ?", id, amount).
		Updates(map[string]any{"remain_quota": gorm.Expr("remain_quota - ?", amount)})
	return res.RowsAffected > 0, res.Error
}

func IncreaseTokenQuota(id int64, amount int64) error {
	if amount <= 0 {
		return nil
	}
	return DB.Model(&Token{}).Where("id = ?", id).
		Updates(map[string]any{"remain_quota": gorm.Expr("remain_quota + ?", amount)}).Error
}

func IncreaseTokenUsedQuota(id int64, amount int64) error {
	if amount <= 0 {
		return nil
	}
	return DB.Model(&Token{}).Where("id = ?", id).
		Updates(map[string]any{"used_quota": gorm.Expr("used_quota + ?", amount)}).Error
}

// ModelLimitList returns the model whitelist as a slice; empty means no limit.
func (t *Token) ModelLimitList() []string {
	return SplitList(t.ModelLimits)
}

// AllowIPList returns the IP whitelist as a slice; empty means no limit.
func (t *Token) AllowIPList() []string {
	return SplitList(t.AllowIps)
}

// AutoGroupList returns the auto-group sequence; empty means no auto groups.
func (t *Token) AutoGroupList() []string {
	return SplitList(t.AutoGroups)
}

// AllowsModel reports whether the token may use the model (ModelLimits empty
// means everything is allowed).
func (t *Token) AllowsModel(model string) bool {
	limits := t.ModelLimitList()
	if len(limits) == 0 {
		return true
	}
	for _, m := range limits {
		if m == model {
			return true
		}
	}
	return false
}

// AllowsIP reports whether the token may be used from the IP (AllowIps empty
// means every IP is allowed). Supports CIDR notation (e.g. 10.0.0.0/8) and
// bare IP addresses. Invalid CIDR entries are treated as exact string matches
// for backward compatibility.
func (t *Token) AllowsIP(ip string) bool {
	ips := t.AllowIPList()
	if len(ips) == 0 {
		return true
	}
	parsedIP := net.ParseIP(ip)
	for _, entry := range ips {
		entry = strings.TrimSpace(entry)
		if entry == ip {
			return true
		}
		// Try CIDR match.
		if strings.Contains(entry, "/") {
			if _, cidr, err := net.ParseCIDR(entry); err == nil {
				if parsedIP != nil && cidr.Contains(parsedIP) {
					return true
				}
			}
		} else {
			// Bare IP — exact match (already checked above, but handle
			// IPv4-mapped IPv6 normalization).
			if parsed := net.ParseIP(entry); parsed != nil && parsedIP != nil {
				if parsed.Equal(parsedIP) {
					return true
				}
			}
		}
	}
	return false
}
