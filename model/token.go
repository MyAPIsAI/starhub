package model

import (
	"errors"
	"strings"

	"gorm.io/gorm"

	"starhub/common"
)

var ErrTokenNotFound = errors.New("token not found")

// AllowsSmartRouting describes existing routing semantics, not an extra grant.
// Virtual-only allowlists cannot authorize any actual upstream model.
func (t *Token) AllowsSmartRouting() bool {
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
	Key            string `gorm:"uniqueIndex;size:64;not null" json:"key"`
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
	CreatedAt  int64  `gorm:"autoCreateTime" json:"created_at"`
}

func InsertToken(t *Token) error {
	var err error
	for i := 0; i < 3; i++ {
		t.Key = common.GenerateTokenKey()
		err = DB.Create(t).Error
		if err == nil {
			return nil
		}
		if !strings.Contains(err.Error(), "UNIQUE constraint failed") {
			return err
		}
	}
	return err
}

func GetTokenByKey(key string) (*Token, error) {
	if key == "" {
		return nil, ErrTokenNotFound
	}
	var t Token
	err := DB.Where("key = ?", key).First(&t).Error
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
	return tokens, err
}

func UpdateTokenFields(id int64, fields map[string]any) error {
	if len(fields) == 0 {
		return nil
	}
	return DB.Model(&Token{}).Where("id = ?", id).Updates(fields).Error
}

func DeleteToken(id int64) error {
	return DB.Delete(&Token{}, "id = ?", id).Error
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
	for _, allowed := range ips {
		if allowed == ip {
			return true
		}
	}
	return false
}
