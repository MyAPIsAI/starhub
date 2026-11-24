package model

import (
	"errors"
	"fmt"
	"math/rand/v2"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"unicode/utf8"

	"gorm.io/gorm"

	"starhub/common"
	"starhub/relay/channel"
)

var ErrChannelNotFound = errors.New("channel not found")
var ErrResourceGatewayManagedChannel = errors.New("resource-gateway managed channels do not support direct model append")
var ErrInvalidChannelModel = errors.New("channel model must contain 1 to 128 characters and cannot contain commas or newlines")

// Multi-key channels store several upstream keys in Key (newline separated)
// and pick one per request according to KeyMode.
const (
	KeyModeRandom     = "random"
	KeyModeRoundRobin = "round_robin"
)

type Channel struct {
	RuntimeKeys            map[string]bool `gorm:"-" json:"-"`
	RuntimeSkipped         []int64         `gorm:"-" json:"-"`
	UsedQuotaRemainderNano int64           `gorm:"not null;default:0" json:"-"`
	RuntimeRoute           *RoutingAttempt `gorm:"-" json:"-"`
	RuntimeModel           string          `gorm:"-" json:"-"`
	RuntimeStage           RoutingStage    `gorm:"-" json:"-"`
	runtimeFallback        *fallbackPermit
	ID                     int64             `gorm:"primaryKey;autoIncrement" json:"id"`
	TaskPluginID           int64             `gorm:"not null;default:0;index" json:"task_plugin_id"`
	Name                   string            `gorm:"size:128;not null;default:''" json:"name"`
	Type                   int               `gorm:"not null;default:1" json:"type"`
	Key                    string            `gorm:"type:text;serializer:channel_credential;not null;default:''" json:"key"`
	KeyMode                string            `gorm:"size:16;not null;default:''" json:"key_mode"`
	MultiKeyStatus         string            `gorm:"size:512;not null;default:''" json:"multi_key_status"`
	BaseURL                string            `gorm:"size:256;not null;default:''" json:"base_url"`
	Models                 string            `gorm:"type:text;not null;default:''" json:"models"`
	ModelCapabilities      map[string]string `gorm:"serializer:json;type:text" json:"model_capabilities"`
	Group                  string            `gorm:"column:group;size:256;not null;default:'default'" json:"group"`
	Status                 int               `gorm:"not null;default:1" json:"status"`
	Priority               int64             `gorm:"not null;default:0" json:"priority"`
	Weight                 int64             `gorm:"not null;default:1" json:"weight"`
	ResponseTime           int64             `gorm:"not null;default:0" json:"response_time"`
	UsedQuota              int64             `gorm:"not null;default:0" json:"used_quota"`
	CreatedAt              int64             `gorm:"autoCreateTime" json:"created_at"`
}

func InsertChannel(c *Channel) error {
	c.Models = strings.Join(uniqueModelNames(SplitList(c.Models)), ",")
	c.pruneModelCapabilities()
	return mutateChannelConfiguration(func(tx *gorm.DB) error {
		if err := validateTaskPluginChannel(tx, c); err != nil {
			return fmt.Errorf("%w: %v", ErrTaskPluginConfiguration, err)
		}
		if err := tx.Create(c).Error; err != nil {
			return err
		}
		if err := rebuildAbilities(tx, c); err != nil {
			return err
		}
		_, err := ensureChannelModelMetadata(tx, SplitList(c.Models))
		return err
	})
}

func UpdateChannel(c *Channel) error {
	c.Models = strings.Join(uniqueModelNames(SplitList(c.Models)), ",")
	c.pruneModelCapabilities()
	return mutateChannelConfiguration(func(tx *gorm.DB) error {
		var previous Channel
		err := tx.Select("id", "type", "key", "base_url", "task_plugin_id").First(&previous, "id = ?", c.ID).Error
		if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		if err == nil {
			if err := guardTaskPluginChannel(tx, &previous, c); err != nil {
				return err
			}
		}
		if err := validateTaskPluginChannel(tx, c); err != nil {
			return fmt.Errorf("%w: %v", ErrTaskPluginConfiguration, err)
		}
		if err == nil && (previous.Type != c.Type || previous.Key != c.Key || previous.BaseURL != c.BaseURL) {
			if err := revokeResourceGatewayChannel(tx, c.ID); err != nil {
				return err
			}
		}
		if err := tx.Save(c).Error; err != nil {
			return err
		}
		if err := rebuildAbilities(tx, c); err != nil {
			return err
		}
		_, err = ensureChannelModelMetadata(tx, SplitList(c.Models))
		return err
	})
}

// AppendChannelModel adds one model without replacing the channel's existing model list.
func AppendChannelModel(channelID int64, name string) (bool, error) {
	name = strings.TrimSpace(name)
	if channelID <= 0 || name == "" || utf8.RuneCountInString(name) > 128 || strings.ContainsAny(name, ",\r\n") {
		return false, ErrInvalidChannelModel
	}
	added := false
	err := mutateChannelConfiguration(func(tx *gorm.DB) error {
		var ch Channel
		if err := tx.First(&ch, "id = ?", channelID).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrChannelNotFound
			}
			return err
		}
		var managed int64
		if err := tx.Model(&ResourceGatewayManagedChannel{}).Where("channel_id = ?", channelID).Count(&managed).Error; err != nil {
			return err
		}
		if managed > 0 {
			return ErrResourceGatewayManagedChannel
		}
		models := SplitList(ch.Models)
		for _, existing := range models {
			if existing == name {
				return nil
			}
		}
		ch.Models = strings.Join(append(models, name), ",")
		ch.pruneModelCapabilities()
		if err := validateTaskPluginChannel(tx, &ch); err != nil {
			return fmt.Errorf("%w: %v", ErrTaskPluginConfiguration, err)
		}
		if err := tx.Model(&Channel{}).Where("id = ?", channelID).Update("models", ch.Models).Error; err != nil {
			return err
		}
		if err := addModelAbilities(tx, &ch, []string{name}); err != nil {
			return err
		}
		if _, err := ensureChannelModelMetadata(tx, []string{name}); err != nil {
			return err
		}
		added = true
		return nil
	})
	return added, err
}

// Capability hints belong only to explicitly configured Ark models, never to authorization or pricing.
func (c *Channel) pruneModelCapabilities() {
	var kept map[string]string
	if c.Type == channel.ChannelTypeArk {
		for _, id := range SplitList(c.Models) {
			switch domain := c.ModelCapabilities[id]; domain {
			case "LLM", "VLM", "ImageGeneration", "VideoGeneration", "3DGeneration", "Audio", "Embedding":
				if kept == nil {
					kept = map[string]string{}
				}
				kept[id] = domain
			}
		}
	}
	c.ModelCapabilities = kept
}

func DeleteChannel(id int64) error {
	return mutateChannelConfiguration(func(tx *gorm.DB) error {
		var previous Channel
		if err := tx.First(&previous, "id = ?", id).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrChannelNotFound
			}
			return err
		}
		if err := guardTaskPluginChannel(tx, &previous, nil); err != nil {
			return err
		}
		res := tx.Delete(&Channel{}, "id = ?", id)
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			return ErrChannelNotFound
		}
		if err := revokeResourceGatewayChannel(tx, id); err != nil {
			return err
		}
		if err := tx.Where("channel_id = ?", id).Delete(&ChannelResourceDisable{}).Error; err != nil {
			return err
		}
		return tx.Where("channel_id = ?", id).Delete(&Ability{}).Error
	})
}

func GetChannelByID(id int64) (*Channel, error) {
	if id <= 0 {
		return nil, ErrChannelNotFound
	}
	var c Channel
	err := DB.First(&c, "id = ?", id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrChannelNotFound
	}
	if err != nil {
		return nil, err
	}
	return &c, nil
}

func ListChannels() ([]Channel, error) {
	var channels []Channel
	err := DB.Order("id ASC").Find(&channels).Error
	return channels, err
}

func UpdateChannelStatus(id int64, status int) error {
	return mutateChannelConfiguration(func(tx *gorm.DB) error {
		if status == common.ChannelStatusEnabled {
			var ch Channel
			if err := tx.Select("id", "type", "task_plugin_id").First(&ch, "id = ?", id).Error; err != nil {
				if errors.Is(err, gorm.ErrRecordNotFound) {
					return ErrChannelNotFound
				}
				return err
			}
			if ch.Type == channel.ChannelTypeTaskPlugin {
				var enabled int64
				if err := tx.Model(&TaskPlugin{}).Where("id = ? AND enabled = ?", ch.TaskPluginID, true).Count(&enabled).Error; err != nil {
					return err
				}
				if enabled == 0 {
					return fmt.Errorf("%w: task plugin version is disabled or unavailable", ErrTaskPluginConfiguration)
				}
			}
		}
		res := tx.Model(&Channel{}).Where("id = ?", id).Updates(map[string]any{"status": status})
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			return ErrChannelNotFound
		}
		if err := tx.Model(&Ability{}).Where("channel_id = ?", id).
			Updates(map[string]any{"enabled": status == common.ChannelStatusEnabled}).Error; err != nil {
			return err
		}
		return applyChannelResourceDisables(tx, id)
	})
}

func UpdateChannelResponseTime(id int64, milliseconds int64) error {
	res := DB.Model(&Channel{}).Where("id = ?", id).
		Updates(map[string]any{"response_time": milliseconds})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ErrChannelNotFound
	}
	return nil
}

func IncreaseChannelUsedQuota(id int64, amount int64) error {
	if amount <= 0 {
		return nil
	}
	return DB.Model(&Channel{}).Where("id = ?", id).
		Updates(map[string]any{"used_quota": gorm.Expr("used_quota + ?", amount)}).Error
}

func SplitList(s string) []string {
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

// Keys returns the upstream keys of the channel; multi-key channels separate
// them with newlines.
func (c *Channel) Keys() []string {
	parts := strings.Split(strings.ReplaceAll(c.Key, "\r\n", "\n"), "\n")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

// IsMultiKey reports whether the channel holds more than one upstream key.
func (c *Channel) IsMultiKey() bool {
	return len(c.Keys()) > 1
}

// KeyAt returns the upstream key at index idx when it exists and is enabled.
// It lets affinity pin a specific account of a multi-key channel.
func (c *Channel) KeyAt(idx int) (string, bool) {
	keys := c.Keys()
	if idx < 0 || idx >= len(keys) {
		return "", false
	}
	statuses := c.KeyStatuses()
	if statuses[idx] != common.StatusEnabled {
		return "", false
	}
	return keys[idx], true
}

// KeyStatuses returns the per-key status list aligned with Keys(); missing or
// malformed entries default to enabled.
func (c *Channel) KeyStatuses() []int {
	keys := c.Keys()
	statuses := make([]int, len(keys))
	for i := range statuses {
		statuses[i] = common.StatusEnabled
	}
	parts := SplitList(c.MultiKeyStatus)
	for i := range parts {
		if i >= len(statuses) {
			break
		}
		if v, err := strconv.Atoi(parts[i]); err == nil {
			statuses[i] = v
		}
	}
	return statuses
}

// multiKeyCounters drives round-robin selection; it is in-memory state, reset
// per process (and per test run via ResetMultiKeyCounters).
var multiKeyCounters sync.Map // channel ID -> *atomic.Int64

// ResetMultiKeyCounters clears the round-robin cursors (used by tests).
func ResetMultiKeyCounters() {
	multiKeyCounters = sync.Map{}
}

// SelectKey returns an enabled upstream key for the channel and its index.
// Multi-key channels pick per KeyMode (weighted random by default, round
// robin when configured) among the enabled keys; single-key channels always
// return their only key. An empty key with index -1 means every key is
// disabled.
func (c *Channel) SelectKey() (string, int) {
	keys := c.Keys()
	if len(keys) <= 1 {
		if len(keys) == 1 {
			return keys[0], 0
		}
		return "", -1
	}
	statuses := c.KeyStatuses()
	enabled := make([]int, 0, len(keys))
	for i, s := range statuses {
		if s == common.StatusEnabled {
			enabled = append(enabled, i)
		}
	}
	if len(enabled) == 0 {
		return "", -1
	}
	if c.KeyMode == KeyModeRoundRobin {
		raw, _ := multiKeyCounters.LoadOrStore(c.ID, new(atomic.Int64))
		counter := raw.(*atomic.Int64)
		for {
			n := counter.Load()
			idx := enabled[int(n)%len(enabled)]
			if counter.CompareAndSwap(n, n+1) {
				return keys[idx], idx
			}
		}
	}
	pick := enabled[rand.IntN(len(enabled))]
	return keys[pick], pick
}

// DisableChannelKey disables a single upstream key of a multi-key channel. It
// reports whether every key is now disabled, in which case the caller should
// disable the whole channel.
func DisableChannelKey(channelID int64, keyIdx int) (allDisabled bool, err error) {
	err = mutateChannelConfiguration(func(tx *gorm.DB) error {
		var ch Channel
		if err := tx.First(&ch, "id = ?", channelID).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrChannelNotFound
			}
			return err
		}
		statuses := ch.KeyStatuses()
		if keyIdx < 0 || keyIdx >= len(statuses) {
			return nil
		}
		statuses[keyIdx] = common.StatusDisabled
		allDisabled = true
		statusList := make([]string, len(statuses))
		for i, status := range statuses {
			statusList[i] = strconv.Itoa(status)
			if status == common.StatusEnabled {
				allDisabled = false
			}
		}
		return tx.Model(&Channel{}).Where("id = ?", channelID).
			Update("multi_key_status", strings.Join(statusList, ",")).Error
	})
	if err != nil {
		return false, err
	}
	return allDisabled, nil
}

func ChannelGroups(c *Channel) []string {
	groups := SplitList(c.Group)
	if len(groups) == 0 {
		groups = []string{common.DefaultGroup}
	}
	return groups
}
