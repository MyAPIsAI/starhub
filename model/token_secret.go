package model

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"gorm.io/gorm"
)

const tokenCipherSetting = "starhub:token-cipher"
const tokenCipherPrefix = "enc:v1:"

// A deployment may supply the same key to each gateway. Otherwise the key is
// persisted beside SQLite and must be backed up separately from the database.
func openTokenCipher(path string) (cipher.AEAD, error) {
	var key []byte
	var err error
	if value := os.Getenv("STARHUB_KEY_ENCRYPTION_KEY"); value != "" {
		key, err = base64.StdEncoding.DecodeString(value)
	} else if path == ":memory:" || strings.Contains(path, "mode=memory") {
		key = make([]byte, 32)
		_, err = rand.Read(key)
	} else {
		keyPath := path + ".key"
		file, createErr := os.OpenFile(keyPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
		if createErr == nil {
			key = make([]byte, 32)
			if _, err = rand.Read(key); err == nil {
				_, err = file.Write(key)
			}
			if closeErr := file.Close(); err == nil {
				err = closeErr
			}
		} else if errors.Is(createErr, os.ErrExist) {
			for attempt := 0; attempt < 100; attempt++ {
				key, err = os.ReadFile(keyPath)
				if err != nil || len(key) == 32 {
					break
				}
				time.Sleep(10 * time.Millisecond)
			}
		} else {
			err = createErr
		}
	}
	if err != nil || len(key) != 32 {
		return nil, errors.New("key encryption requires a readable 32-byte key (STARHUB_KEY_ENCRYPTION_KEY must be base64)")
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	return cipher.NewGCM(block)
}

func tokenKeyHash(key string) string {
	digest := sha256.Sum256([]byte(key))
	return hex.EncodeToString(digest[:])
}

func tokenCipher(tx *gorm.DB) (cipher.AEAD, error) {
	value, ok := tx.Get(tokenCipherSetting)
	if !ok {
		return nil, errors.New("key encryption is not initialized")
	}
	return value.(cipher.AEAD), nil
}

func sealTokenKey(tx *gorm.DB, key string) (string, error) {
	aead, err := tokenCipher(tx)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return "", err
	}
	sealed := aead.Seal(nonce, nonce, []byte(key), []byte(tokenKeyHash(key)))
	return tokenCipherPrefix + base64.RawStdEncoding.EncodeToString(sealed), nil
}

func (t *Token) BeforeCreate(tx *gorm.DB) error {
	initializeTokenPeriod(t)
	if err := validateTokenPeriod(t.QuotaPeriod, t.QuotaLimit); err != nil {
		return err
	}
	if t.Key == "" {
		return errors.New("key must not be empty")
	}
	var err error
	t.KeyCiphertext, err = sealTokenKey(tx, t.Key)
	t.KeyHash = tokenKeyHash(t.Key)
	return err
}

func (t *Token) AfterFind(tx *gorm.DB) error {
	aead, err := tokenCipher(tx)
	if err != nil {
		return err
	}
	if !strings.HasPrefix(t.KeyCiphertext, tokenCipherPrefix) {
		return errors.New("unencrypted key encountered after migration")
	}
	sealed, err := base64.RawStdEncoding.DecodeString(strings.TrimPrefix(t.KeyCiphertext, tokenCipherPrefix))
	if err != nil || len(sealed) < aead.NonceSize() {
		return errors.New("invalid encrypted key")
	}
	plain, err := aead.Open(nil, sealed[:aead.NonceSize()], sealed[aead.NonceSize():], []byte(t.KeyHash))
	if err != nil {
		return errors.New("key decryption failed; check the deployment encryption key")
	}
	t.Key = string(plain)
	return nil
}

func encryptTokenFields(tx *gorm.DB, fields map[string]any) error {
	key, ok := fields["key"].(string)
	if !ok {
		return nil
	}
	sealed, err := sealTokenKey(tx, key)
	if err != nil {
		return err
	}
	fields["key"], fields["key_hash"] = sealed, tokenKeyHash(key)
	return nil
}

func migrateTokenSecrets(db *gorm.DB) error {
	return db.Transaction(func(tx *gorm.DB) error {
		// Serialize concurrent gateway startups before reading legacy rows.
		if err := tx.Exec("UPDATE tokens SET id = id WHERE id = (SELECT MIN(id) FROM tokens)").Error; err != nil {
			return err
		}
		var rows []struct {
			ID      int64
			Key     string
			KeyHash string
		}
		if err := tx.Table("tokens").Select("id, key, key_hash").Find(&rows).Error; err != nil {
			return err
		}
		for _, row := range rows {
			if strings.HasPrefix(row.Key, tokenCipherPrefix) {
				token := Token{KeyCiphertext: row.Key, KeyHash: row.KeyHash}
				if err := token.AfterFind(tx); err != nil {
					return fmt.Errorf("validate encrypted key %d: %w", row.ID, err)
				}
				continue
			}
			fields := map[string]any{"key": row.Key}
			if err := encryptTokenFields(tx, fields); err != nil {
				return err
			}
			if err := tx.Table("tokens").Where("id = ?", row.ID).Updates(fields).Error; err != nil {
				return err
			}
		}
		return nil
	})
}
