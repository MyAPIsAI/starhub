package model

import (
	"fmt"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"starhub/setting"
)

type Option struct {
	Key   string `gorm:"primaryKey;size:64" json:"key"`
	Value string `gorm:"type:text;not null;default:''" json:"value"`
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
	opts, err := AllOptions()
	if err != nil {
		return err
	}
	for _, o := range opts {
		if !setting.IsOptionKey(o.Key) {
			continue
		}
		if err := setting.UpdateOption(o.Key, o.Value); err != nil {
			return fmt.Errorf("load option %s: %w", o.Key, err)
		}
	}
	return nil
}
