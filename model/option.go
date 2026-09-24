package model

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"log"
	"sync"
	"sync/atomic"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"starhub/setting"
)

type Option struct {
	Key   string `gorm:"primaryKey;size:64" json:"key"`
	Value string `gorm:"type:text;not null;default:''" json:"value"`
}

type OptionRevision struct {
	ID      int64 `gorm:"primaryKey"`
	Version int64 `gorm:"not null;default:0"`
}

type OptionChange struct {
	ID        int64  `gorm:"primaryKey;autoIncrement" json:"id"`
	Version   int64  `gorm:"uniqueIndex;not null" json:"version"`
	Key       string `gorm:"size:64;not null" json:"key"`
	OldValue  string `gorm:"type:text" json:"old_value"`
	NewValue  string `gorm:"type:text" json:"new_value"`
	ActorID   int64  `gorm:"not null;default:0" json:"actor_id"`
	CreatedAt int64  `gorm:"autoCreateTime" json:"created_at"`
}

type OptionInstance struct {
	ID       string `gorm:"primaryKey;size:32" json:"id"`
	Version  int64  `gorm:"not null" json:"version"`
	LastSeen int64  `gorm:"index;not null" json:"last_seen"`
}

var loadedOptionVersion atomic.Int64
var optionLoadMu sync.Mutex
var ErrOptionVersionConflict = errors.New("option version changed")

func OptionVersion() (int64, error) {
	var revision OptionRevision
	if err := DB.First(&revision, 1).Error; err != nil {
		return 0, err
	}
	return revision.Version, nil
}

func LoadedOptionVersion() int64 { return loadedOptionVersion.Load() }

func RefreshOptionsOnce() (bool, error) {
	version, err := OptionVersion()
	if err != nil || version == LoadedOptionVersion() {
		return false, err
	}
	return true, LoadOptionsToSettings()
}

// WatchOptions reloads changes made by another process using the same store.
func WatchOptions(ctx context.Context, interval time.Duration) {
	identity := make([]byte, 12)
	if _, err := rand.Read(identity); err != nil {
		log.Printf("option watcher identity failed: %v", err)
		return
	}
	id := hex.EncodeToString(identity)
	if err := DB.Where("last_seen < ?", time.Now().Add(-24*time.Hour).Unix()).
		Delete(&OptionInstance{}).Error; err != nil {
		log.Printf("option instance cleanup failed: %v", err)
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	lastBeat := time.Time{}
	lastReportedVersion := int64(-1)
	for {
		if time.Since(lastBeat) >= 5*time.Second || lastReportedVersion != LoadedOptionVersion() {
			if err := recordOptionInstance(id); err != nil {
				log.Printf("option instance heartbeat failed: %v", err)
			} else {
				lastBeat = time.Now()
				lastReportedVersion = LoadedOptionVersion()
			}
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			changed, err := RefreshOptionsOnce()
			if err != nil {
				log.Printf("option reload failed: %v", err)
			} else if changed {
				if err := recordOptionInstance(id); err != nil {
					log.Printf("option instance version update failed: %v", err)
				}
			}
		}
	}
}

func recordOptionInstance(id string) error {
	instance := OptionInstance{ID: id, Version: LoadedOptionVersion(), LastSeen: time.Now().Unix()}
	return DB.Clauses(clause.OnConflict{UpdateAll: true}).Create(&instance).Error
}

func ActiveOptionInstances() ([]OptionInstance, error) {
	var instances []OptionInstance
	err := DB.Where("last_seen >= ?", time.Now().Add(-15*time.Second).Unix()).
		Order("id ASC").Find(&instances).Error
	return instances, err
}

// SaveOption commits the value and its change record as one database version.
func SaveOption(key, value string, actorID int64) (int64, error) {
	return saveOption(key, value, actorID, -1)
}

func SaveOptionIfVersion(key, value string, actorID, expectedVersion int64) (int64, error) {
	return saveOption(key, value, actorID, expectedVersion)
}

func saveOption(key, value string, actorID, expectedVersion int64) (int64, error) {
	normalized, err := setting.NormalizeOption(key, value)
	if err != nil {
		return 0, err
	}
	value = normalized
	var version int64
	err = DB.Transaction(func(tx *gorm.DB) error {
		if expectedVersion >= 0 {
			var revision OptionRevision
			if err := tx.First(&revision, 1).Error; err != nil {
				return err
			}
			if revision.Version != expectedVersion {
				return fmt.Errorf("%w: current %d", ErrOptionVersionConflict, revision.Version)
			}
		}
		var old Option
		err := tx.Where("`key` = ?", key).First(&old).Error
		if err != nil && err != gorm.ErrRecordNotFound {
			return err
		}
		if err == nil && old.Value == value {
			var revision OptionRevision
			if err := tx.First(&revision, 1).Error; err != nil {
				return err
			}
			version = revision.Version
			return nil
		}
		if err := tx.Clauses(clause.OnConflict{UpdateAll: true}).Create(&Option{Key: key, Value: value}).Error; err != nil {
			return err
		}
		if err := tx.Model(&OptionRevision{}).Where("id = 1").
			Update("version", gorm.Expr("version + 1")).Error; err != nil {
			return err
		}
		var revision OptionRevision
		if err := tx.First(&revision, 1).Error; err != nil {
			return err
		}
		version = revision.Version
		before, after := old.Value, value
		if key == setting.OptionKeyAlertWebhookURL {
			before, after = "[redacted]", "[redacted]"
		}
		return tx.Create(&OptionChange{Version: version, Key: key, OldValue: before,
			NewValue: after, ActorID: actorID, CreatedAt: time.Now().Unix()}).Error
	})
	return version, err
}

func ListOptionChanges(limit int) ([]OptionChange, error) {
	if limit < 1 || limit > 100 {
		limit = 50
	}
	var changes []OptionChange
	err := DB.Order("version DESC").Limit(limit).Find(&changes).Error
	return changes, err
}

func RollbackOption(changeVersion, expectedVersion, actorID int64) (int64, error) {
	var change OptionChange
	if err := DB.Where("version = ?", changeVersion).First(&change).Error; err != nil {
		return 0, err
	}
	if change.Key == setting.OptionKeyAlertWebhookURL {
		return 0, fmt.Errorf("webhook URL history is redacted and cannot be restored")
	}
	value, err := setting.NormalizeOption(change.Key, change.OldValue)
	if err != nil {
		return 0, err
	}
	return saveOption(change.Key, value, actorID, expectedVersion)
}

func GetOption(key string) (string, bool, error) {
	var o Option
	err := DB.Where(map[string]any{"key": key}).First(&o).Error
	if err == gorm.ErrRecordNotFound {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	return o.Value, true, nil
}

func UpsertOption(key, value string) error {
	return DB.Clauses(clause.OnConflict{UpdateAll: true}).
		Create(&Option{Key: key, Value: value}).Error
}

func AllOptions() ([]Option, error) {
	var opts []Option
	err := DB.Order("`key` ASC").Find(&opts).Error
	return opts, err
}

func SeedOptions() error {
	for _, key := range setting.OptionKeys() {
		_, found, err := GetOption(key)
		if err != nil {
			return err
		}
		if found {
			continue
		}
		snapshot, err := setting.SnapshotOption(key)
		if err != nil {
			return err
		}
		if err := UpsertOption(key, snapshot); err != nil {
			return err
		}
	}
	return nil
}

func LoadOptionsToSettings() error {
	optionLoadMu.Lock()
	defer optionLoadMu.Unlock()
	version, err := OptionVersion()
	if err != nil {
		return err
	}
	opts, err := AllOptions()
	if err != nil {
		return err
	}
	latest, err := OptionVersion()
	if err != nil {
		return err
	}
	if latest != version {
		return fmt.Errorf("option version changed during reload: %d to %d", version, latest)
	}
	values := make(map[string]string, len(opts))
	for _, o := range opts {
		if !setting.IsOptionKey(o.Key) {
			continue
		}
		if _, err := setting.NormalizeOption(o.Key, o.Value); err != nil {
			return fmt.Errorf("validate option %s: %w", o.Key, err)
		}
		values[o.Key] = o.Value
	}
	if err := setting.ApplyOptions(values); err != nil {
		return err
	}
	for _, o := range opts {
		if !setting.IsOptionKey(o.Key) {
			continue
		}
		// Persist the effective ratio map so older databases and the options
		// API include newly introduced built-in models as well.
		if o.Key == setting.OptionKeyModelRatio || o.Key == setting.OptionKeyCompletionRatio {
			value, err := setting.SnapshotRatio(o.Key)
			if err != nil {
				return err
			}
			if value != o.Value {
				if err := UpsertOption(o.Key, value); err != nil {
					return err
				}
			}
		}
	}
	loadedOptionVersion.Store(version)
	return nil
}
