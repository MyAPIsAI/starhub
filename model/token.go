package model

import (
	"errors"
	"net/netip"
	"strings"
	"time"

	"gorm.io/gorm"

	"starhub/common"
)

var (
	ErrTokenNotFound     = errors.New("token not found")
	ErrTokenLimitReached = errors.New("key limit reached: maximum 200 keys per account")
)

const MaxTokensPerUser = 200

// AllowsSmartRouting honors explicit policy while preserving legacy key behavior.
// Virtual-only allowlists cannot authorize any actual upstream model.
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
	ID              int64  `gorm:"primaryKey;autoIncrement" json:"id"`
	UserID          int64  `gorm:"index;not null" json:"user_id"`
	Name            string `gorm:"size:64;not null;default:''" json:"name"`
	Key             string `gorm:"-" json:"key"`
	KeyCiphertext   string `gorm:"column:key;not null" json:"-"`
	KeyHash         string `gorm:"uniqueIndex;size:64" json:"-"`
	Status          int    `gorm:"not null;default:1" json:"status"`
	Group           string `gorm:"column:group;size:64;not null;default:''" json:"group"`
	ExpiredTime     int64  `gorm:"not null;default:-1" json:"expired_time"`
	RemainQuota     int64  `gorm:"not null;default:0" json:"remain_quota"`
	UnlimitedQuota  bool   `gorm:"not null;default:false" json:"unlimited_quota"`
	UsedQuota       int64  `gorm:"not null;default:0" json:"used_quota"`
	QuotaPeriod     string `gorm:"not null;default:'never'" json:"quota_period"`
	QuotaLimit      int64  `gorm:"not null;default:0" json:"quota_limit"`
	QuotaTimezone   string `gorm:"not null;default:'Asia/Dubai'" json:"quota_timezone"`
	PeriodUsedQuota int64  `gorm:"not null;default:0" json:"period_used_quota"`
	PeriodStartedAt int64  `gorm:"not null;default:0" json:"period_started_at"`
	// ModelLimits is a comma-separated model whitelist; empty means no limit.
	// A sub-key can be restricted to cheaper models only.
	ModelLimits string `gorm:"size:1024;not null;default:''" json:"model_limits"`
	// Nil preserves the routing permission of keys created before explicit smart policies.
	SmartAccess *bool `json:"smart_access,omitempty"`
	// Empty inherits the model allowlist; a nonempty list only narrows smart routing.
	SmartCandidates string `gorm:"size:1024;not null;default:''" json:"smart_candidates"`
	// AllowIps is a comma-separated IP whitelist; empty means no limit.
	// Enterprise customers use it to prevent key leakage.
	AllowIps string `gorm:"size:4096;not null;default:''" json:"allow_ips"`
	// AutoGroups is a comma-separated group sequence tried in order when the
	// token's group is "auto" (cheapest-first, degrade to pricier on failure).
	AutoGroups string `gorm:"size:512;not null;default:''" json:"auto_groups"`
	CreatedAt  int64  `gorm:"autoCreateTime" json:"created_at"`
	Revision   int64  `gorm:"not null;default:0" json:"management_version"`
}

func InsertToken(t *Token) error {
	var err error
	t.AllowIps, err = NormalizeTokenIPs(t.AllowIps)
	if err != nil {
		return err
	}
	if err := ValidateTokenSmartPolicy(t); err != nil {
		return err
	}
	return DB.Transaction(func(tx *gorm.DB) error {
		if err := checkTokenCreationLimit(tx, t.UserID); err != nil {
			return err
		}
		var err error
		for i := 0; i < 3; i++ {
			t.Key = common.GenerateTokenKey()
			err = tx.Create(t).Error
			if err == nil || !strings.Contains(err.Error(), "UNIQUE constraint failed") {
				return err
			}
		}
		return err
	})
}

func checkTokenCreationLimit(tx *gorm.DB, owner int64) error {
	// Claim the SQLite writer before counting, including when this owner has no keys.
	if err := tx.Model(&User{}).Where("id = ?", owner).UpdateColumn("id", gorm.Expr("id")).Error; err != nil {
		return err
	}
	var count int64
	if err := tx.Model(&Token{}).Where("user_id = ?", owner).Count(&count).Error; err != nil {
		return err
	}
	if count >= MaxTokensPerUser {
		return ErrTokenLimitReached
	}
	return nil
}

func ValidateTokenSmartPolicy(t *Token) error {
	if len(t.SmartCandidates) > 1024 {
		return errors.New("smart candidate list exceeds limit")
	}
	seen := map[string]bool{}
	candidates := SplitList(t.SmartCandidates)
	if t.SmartCandidates != "" && len(candidates) == 0 {
		return errors.New("smart candidates must contain a model")
	}
	for _, name := range candidates {
		if !concreteResourceModel(name) || seen[name] || (t.ModelLimits != "" && !containsTokenModel(t.ModelLimitList(), name)) {
			return errors.New("smart candidates must be unique concrete models within the key allowlist")
		}
		seen[name] = true
	}
	return nil
}

func containsTokenModel(models []string, name string) bool {
	for _, allowed := range models {
		if allowed == name {
			return true
		}
	}
	return false
}

func GetOwnedTokenByID(owner, id int64) (*Token, error) {
	var token Token
	err := DB.Where("id = ? AND user_id = ?", id, owner).First(&token).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrTokenNotFound
	}
	return &token, err
}

func GetTokenByKey(key string) (*Token, error) {
	if key == "" {
		return nil, ErrTokenNotFound
	}
	var t Token
	err := DB.Where("key_hash = ?", tokenKeyHash(key)).First(&t).Error
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

func ListTokens(userID int64) ([]Token, error) {
	var tokens []Token
	q := DB.Model(&Token{})
	if userID > 0 {
		q = q.Where("user_id = ?", userID)
	}
	err := q.Order("id DESC").Find(&tokens).Error
	if err == nil {
		for i := range tokens {
			if err = RefreshTokenPeriod(&tokens[i], time.Now()); err != nil {
				break
			}
		}
	}
	return tokens, err
}

func UpdateTokenFields(id int64, fields map[string]any) error {
	if len(fields) == 0 {
		return nil
	}
	fields["revision"] = gorm.Expr("revision + 1")
	if err := encryptTokenFields(DB, fields); err != nil {
		return err
	}
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
// means every IP is allowed).
func (t *Token) AllowsIP(ip string) bool {
	ips := t.AllowIPList()
	if len(ips) == 0 {
		return true
	}
	address, err := netip.ParseAddr(ip)
	if err != nil || address.Zone() != "" {
		return false
	}
	for _, allowed := range ips {
		prefix, err := common.ParseIPNetwork(allowed)
		if err == nil && prefix.Contains(address.Unmap()) {
			return true
		}
	}
	return false
}
