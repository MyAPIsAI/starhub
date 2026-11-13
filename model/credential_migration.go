package model

import (
	"errors"
	"strings"

	"gorm.io/gorm"
	"starhub/common"
)

// MigrateCredentialStorage atomically upgrades legacy values or rewraps all
// credentials with the active key. Raw table access avoids double serialization.
func MigrateCredentialStorage(rotate bool, actor, reason string) (int, error) {
	if strings.TrimSpace(actor) == "" || len(actor) > 64 || strings.TrimSpace(reason) == "" || len(reason) > 512 {
		return 0, errors.New("credential migration requires an operator and audit reason")
	}
	changed := 0
	err := DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec("UPDATE channels SET id = id WHERE id = -1").Error; err != nil {
			return err
		}
		for _, target := range []struct {
			table, purpose string
			hash           bool
		}{{"channels", "upstream", false}, {"tokens", "customer", true}} {
			cursor := int64(0)
			for {
				var rows []struct {
					ID      int64
					Key     string
					KeyHash string
				}
				columns := "id, key"
				if target.hash {
					columns += ", key_hash"
				}
				if err := tx.Table(target.table).Select(columns).Where("id > ?", cursor).Order("id").Limit(500).Find(&rows).Error; err != nil {
					return err
				}
				if len(rows) == 0 {
					break
				}
				for _, row := range rows {
					cursor = row.ID
					if row.Key == "" {
						continue
					}
					plaintext, err := common.DecryptCredential(row.Key, target.purpose)
					if err != nil {
						return errors.New("credential migration could not decrypt a stored value")
					}
					hash := ""
					if target.hash {
						hash = common.HashKey(plaintext)
						if row.KeyHash != "" && row.KeyHash != hash {
							return errors.New("customer lookup secret does not match stored key identity")
						}
					}
					if !rotate && strings.HasPrefix(row.Key, common.CredentialEnvelopePrefix) && (!target.hash || row.KeyHash == hash) {
						continue
					}
					encrypted, err := common.EncryptCredential(plaintext, target.purpose)
					if err != nil {
						return err
					}
					values := map[string]any{"key": encrypted}
					if target.hash {
						values["key_hash"] = hash
					}
					result := tx.Table(target.table).Where("id = ? AND key = ?", row.ID, row.Key).UpdateColumns(values)
					if result.Error != nil {
						return result.Error
					}
					if result.RowsAffected != 1 {
						return errors.New("credential changed during migration")
					}
					changed++
				}
			}
		}
		if changed == 0 {
			return nil
		}
		operation := "migrate-credentials"
		if rotate {
			operation = "rotate-credentials"
		}
		return tx.Create(&AuditLog{Username: actor, Method: "LOCAL", Path: "/operator/" + operation, Status: 200, Reason: reason}).Error
	})
	if err != nil {
		return 0, err
	}
	return changed, nil
}
