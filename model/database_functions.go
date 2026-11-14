package model

import "gorm.io/gorm"

func initDatabaseFunctions(db *gorm.DB) error {
	if db.Dialector.Name() != "postgres" {
		return nil
	}
	// PostgreSQL 15 has no safe text-to-JSON predicate. Malformed persisted
	// capability rows must be excluded rather than aborting channel selection.
	return db.Exec(`CREATE OR REPLACE FUNCTION starhub_valid_jsonb(value text) RETURNS jsonb
LANGUAGE plpgsql IMMUTABLE STRICT AS $$
BEGIN RETURN value::jsonb;
EXCEPTION WHEN invalid_text_representation OR untranslatable_character THEN RETURN NULL;
END $$`).Error
}
