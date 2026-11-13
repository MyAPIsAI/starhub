package model

import (
	"context"
	"errors"
	"reflect"

	"gorm.io/gorm"
	"gorm.io/gorm/schema"
	"starhub/common"
)

type channelCredentialSerializer struct{}

func init() { schema.RegisterSerializer("channel_credential", channelCredentialSerializer{}) }

func (channelCredentialSerializer) Scan(ctx context.Context, field *schema.Field, dst reflect.Value, value any) error {
	stored := ""
	switch raw := value.(type) {
	case nil:
	case string:
		stored = raw
	case []byte:
		stored = string(raw)
	default:
		return errors.New("invalid stored upstream credential")
	}
	plaintext, err := common.DecryptCredential(stored, "upstream")
	if err != nil {
		return errors.New("upstream credential could not be decrypted")
	}
	field.ReflectValueOf(ctx, dst).SetString(plaintext)
	return nil
}

func (channelCredentialSerializer) Value(_ context.Context, _ *schema.Field, _ reflect.Value, value any) (any, error) {
	plaintext, ok := value.(string)
	if !ok {
		return nil, errors.New("invalid upstream credential")
	}
	return common.EncryptCredential(plaintext, "upstream")
}

// Map updates bypass field serializers in GORM. Apply encryption explicitly
// while keeping struct reads and writes plaintext only in process memory.
func (channel *Channel) BeforeSave(tx *gorm.DB) error {
	if values, ok := tx.Statement.Dest.(map[string]any); ok {
		for _, name := range []string{"key", "Key"} {
			value, exists := values[name]
			if !exists {
				continue
			}
			plaintext, ok := value.(string)
			if !ok {
				return errors.New("invalid upstream credential update")
			}
			encrypted, err := common.EncryptCredential(plaintext, "upstream")
			if err != nil {
				return err
			}
			values[name] = encrypted
		}
	}
	return nil
}
