package model

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"gorm.io/gorm"
	"starhub/common"
	"strconv"
	"strings"
)

var ErrOwnedTokenConflict = errors.New("owned token state changed")

type OwnedTokenChoice struct {
	Group     string   `json:"group"`
	Model     string   `json:"model"`
	Protocols []string `json:"protocols"`
}
type OwnedTokenChoices struct {
	UserID   int64              `json:"user_id"`
	Revision string             `json:"revision"`
	Choices  []OwnedTokenChoice `json:"choices"`
}
type OwnedTokenReceipt struct {
	ID          int64  `gorm:"primaryKey" json:"-"`
	UserID      int64  `gorm:"uniqueIndex:idx_owned_token_operation;not null" json:"user_id"`
	OperationID string `gorm:"uniqueIndex:idx_owned_token_operation;size:32;not null" json:"operation_id"`
	Action      string `json:"action"`
	TokenID     int64  `json:"token_id"`
	Revision    string `json:"revision"`
	CreatedAt   int64  `gorm:"autoCreateTime" json:"created_at"`
}

func tokenDigest(v any) string {
	raw, _ := json.Marshal(v)
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}
func TokenManagementRevision(t *Token) string { return tokenDigest(t) }
func ReadOwnedTokenChoices(owner int64) (*OwnedTokenChoices, error) {
	var out *OwnedTokenChoices
	err := DB.Transaction(func(tx *gorm.DB) error { var err error; out, err = readOwnedTokenChoices(tx, owner); return err })
	return out, err
}
func readOwnedTokenChoices(tx *gorm.DB, owner int64) (*OwnedTokenChoices, error) {
	var user User
	if err := tx.First(&user, "id = ? AND status = ?", owner, common.StatusEnabled).Error; err != nil {
		return nil, err
	}
	var abilities []Ability
	if err := tx.Where("enabled = ?", true).Order("\"group\", model, channel_id").Find(&abilities).Error; err != nil {
		return nil, err
	}
	var definitions []ServiceGroupConfig
	if err := tx.Order("group_id").Find(&definitions).Error; err != nil {
		return nil, err
	}
	var deleted []ServiceGroupDeletion
	if err := tx.Order("group_id").Find(&deleted).Error; err != nil {
		return nil, err
	}
	var options OptionRevision
	var channels ChannelConfigurationState
	if err := tx.First(&options, 1).Error; err != nil {
		return nil, err
	}
	if err := tx.First(&channels, 1).Error; err != nil {
		return nil, err
	}
	ratios := map[string]float64{"default": 1}
	var option Option
	err := tx.First(&option, "key = ?", "group_ratio").Error
	if err == nil {
		if json.Unmarshal([]byte(option.Value), &ratios) != nil {
			return nil, errors.New("invalid group ratio")
		}
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}
	allowed := map[string]bool{}
	for group := range ratios {
		allowed[group] = true
	}
	for _, d := range definitions {
		ok := d.Enabled && d.Visibility == "common"
		if d.Enabled && d.Visibility == "custom" {
			for _, id := range SplitList(d.AccountIDs) {
				ok = ok || id == strconv.FormatInt(owner, 10)
			}
		}
		allowed[d.GroupID] = allowed[d.GroupID] && ok
	}
	for _, d := range deleted {
		allowed[d.GroupID] = false
	}
	out := &OwnedTokenChoices{UserID: owner, Choices: []OwnedTokenChoice{}}
	seen := map[string]bool{}
	for _, a := range abilities {
		key := a.Group + "\x00" + a.Model
		if allowed[a.Group] && !seen[key] && a.Group != "auto" && a.Model != "myapis-auto" && a.Model != "veyra-auto" {
			out.Choices = append(out.Choices, OwnedTokenChoice{Group: a.Group, Model: a.Model, Protocols: []string{}})
			seen[key] = true
		}
	}
	out.Revision = tokenDigest([]any{user.ID, user.Group, user.Role, user.Status, user.DeactivationVersion, options.Version, channels.Revision, definitions, deleted, out.Choices})
	return out, nil
}

// MutateOwnedToken serializes ownership, exact row/config snapshots and the
// durable operation receipt. Plaintext keys are never stored in receipts.
func MutateOwnedToken(owner int64, operation, action string, id int64, revision, choiceRevision string, fields map[string]any) (*Token, *OwnedTokenReceipt, error) {
	var result Token
	var receipt OwnedTokenReceipt
	err := DB.Transaction(func(tx *gorm.DB) error {
		lock := tx.Model(&User{}).Where("id = ? AND status = ?", owner, 1).UpdateColumn("id", gorm.Expr("id"))
		if lock.Error != nil {
			return lock.Error
		}
		if lock.RowsAffected != 1 {
			return ErrOwnedTokenConflict
		}
		var count int64
		if err := tx.Model(&OwnedTokenReceipt{}).Where("user_id = ? AND operation_id = ?", owner, operation).Count(&count).Error; err != nil {
			return err
		}
		if count != 0 {
			return ErrOwnedTokenConflict
		}
		choices, err := readOwnedTokenChoices(tx, owner)
		if err != nil {
			return err
		}
		if choices.Revision != choiceRevision {
			return ErrOwnedTokenConflict
		}
		if action != "create" {
			if err := tx.First(&result, "id = ? AND user_id = ?", id, owner).Error; err != nil {
				return ErrOwnedTokenConflict
			}
			if TokenManagementRevision(&result) != revision {
				return ErrOwnedTokenConflict
			}
		} else {
			result = Token{UserID: owner, Status: 1, ExpiredTime: -1, Key: common.GenerateTokenKey()}
		}
		if action == "create" || action == "update" {
			group := result.Group
			if v, ok := fields["group"]; ok {
				group = v.(string)
			}
			if group == "" {
				var u User
				if err := tx.First(&u, owner).Error; err != nil {
					return err
				}
				group = u.Group
				if group == "" {
					group = "default"
				}
			}
			groups := []string{group}
			if group == "auto" {
				auto := result.AutoGroups
				if v, ok := fields["auto_groups"]; ok {
					auto = v.(string)
				}
				groups = SplitList(auto)
				if len(groups) == 0 {
					return fmt.Errorf("auto group requires groups")
				}
			}
			_, groupChanged := fields["group"]
			_, autoChanged := fields["auto_groups"]
			limits, limitsChanged := fields["model_limits"]
			if action == "create" || groupChanged || autoChanged || limitsChanged {
				models := result.ModelLimits
				if limitsChanged {
					models = limits.(string)
				}
				if action == "create" && models == "" {
					return fmt.Errorf("select explicit models for a new key")
				}
				for _, g := range groups {
					exists := false
					for _, c := range choices.Choices {
						if c.Group == g {
							exists = true
						}
					}
					if !exists {
						return fmt.Errorf("group is unavailable")
					}
					for _, m := range SplitList(models) {
						found := false
						for _, c := range choices.Choices {
							if c.Group == g && c.Model == m {
								found = true
							}
						}
						if !found {
							return fmt.Errorf("model is unavailable in the selected group")
						}
					}
				}
			}
		}
		switch action {
		case "create":
			data := map[string]any{"user_id": owner, "key": result.Key, "status": 1, "expired_time": int64(-1)}
			for k, v := range fields {
				data[k] = v
			}
			if err := tx.Model(&Token{}).Create(data).Error; err != nil {
				return err
			}
			if err := tx.First(&result, "key = ?", result.Key).Error; err != nil {
				return err
			}
		case "update", "regenerate":
			updates := map[string]any{}
			for k, v := range fields {
				updates[k] = v
			}
			// When the lifetime limit is being edited, subtract the
			// in-flight pre-charge held by pending tasks. Without this,
			// remain_quota is set to (new_total - used_quota) which
			// ignores pending reservations; when those tasks settle,
			// the cumulative (remain + used) silently inflates.
			if rq, ok := updates["remain_quota"]; ok && action == "update" {
				pending, err := SumPendingTaskQuota(tx, id)
				if err != nil {
					return err
				}
				if pending > 0 {
					newRemain := rq.(int64) - pending
					if newRemain < 0 {
						return fmt.Errorf("quota limit is below in-flight task reservations (%d)", pending)
					}
					updates["remain_quota"] = newRemain
				}
			}
			if action == "regenerate" {
				updates["key"] = common.GenerateTokenKey()
			}
			updates["revision"] = gorm.Expr("revision + 1")
			if err := tx.Model(&Token{}).Where("id = ? AND user_id = ?", id, owner).Updates(updates).Error; err != nil {
				return err
			}
			if err := tx.First(&result, id).Error; err != nil {
				return err
			}
		case "delete":
			if err := tx.Delete(&Token{}, "id = ? AND user_id = ?", id, owner).Error; err != nil {
				return err
			}
			if err := tx.Where("token_id = ?", id).Delete(&RoutingChain{}).Error; err != nil {
				return err
			}
		default:
			return fmt.Errorf("invalid owned token action")
		}
		receipt = OwnedTokenReceipt{UserID: owner, OperationID: operation, Action: action, TokenID: result.ID, Revision: TokenManagementRevision(&result)}
		return tx.Create(&receipt).Error
	})
	if err != nil {
		return nil, nil, err
	}
	return &result, &receipt, nil
}
func ReadOwnedTokenReceipt(owner int64, operation string) (*OwnedTokenReceipt, error) {
	var receipt OwnedTokenReceipt
	err := DB.Where("user_id = ? AND operation_id = ?", owner, operation).First(&receipt).Error
	return &receipt, err
}
func ValidOwnedTokenFields(fields map[string]any) error {
	for key, value := range fields {
		switch key {
		case "name":
			v := value.(string)
			if strings.TrimSpace(v) == "" || len(v) > 64 {
				return fmt.Errorf("invalid key name")
			}
		case "remain_quota":
			if value.(int64) < 0 {
				return fmt.Errorf("invalid key quota")
			}
		case "status":
			if value.(int) != 1 && value.(int) != 2 {
				return fmt.Errorf("invalid key status")
			}
		case "expired_time":
			if value.(int64) < -1 {
				return fmt.Errorf("invalid key expiry")
			}
		}
	}
	return nil
}
