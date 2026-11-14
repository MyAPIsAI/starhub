package model

import (
	"fmt"

	"gorm.io/gorm"
	"gorm.io/gorm/schema"
)

// Run only during initialization, before publishing the connection to workers.
// MySQL requires expression defaults for TEXT. Restore the cached field metadata
// afterwards so GORM's runtime upserts still include these ordinary defaults.
func migrateDatabaseSchema(db *gorm.DB, values ...any) error {
	if db.Dialector.Name() != "mysql" {
		return db.AutoMigrate(values...)
	}
	type original struct {
		field   *schema.Field
		value   string
		literal any
	}
	var restore []original
	defer func() {
		for _, old := range restore {
			old.field.DefaultValue = old.value
			old.field.DefaultValueInterface = old.literal
		}
	}()
	for _, value := range values {
		stmt := &gorm.Statement{DB: db}
		if err := stmt.Parse(value); err != nil {
			return err
		}
		for _, field := range stmt.Schema.Fields {
			if field.HasDefaultValue && field.DefaultValueInterface != nil && field.DataType == "text" {
				restore = append(restore, original{field, field.DefaultValue, field.DefaultValueInterface})
				field.DefaultValue = fmt.Sprintf("(%s)", db.Dialector.Explain("?", field.DefaultValueInterface))
				field.DefaultValueInterface = nil
			}
		}
	}
	return db.AutoMigrate(values...)
}
