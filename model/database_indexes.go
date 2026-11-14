package model

import "gorm.io/gorm"

// Empty legacy values are not ownership keys. MySQL functional indexes use
// NULL for them, matching the partial unique indexes on SQLite/PostgreSQL.
func createNonemptyUniqueIndex(db *gorm.DB, model any, table, name, column string) error {
	if db.Migrator().HasIndex(model, name) {
		return nil
	}
	key := productionQuote(db, column)
	sql := "CREATE UNIQUE INDEX " + name + " ON " + table
	if db.Dialector.Name() == "mysql" {
		sql += " ((NULLIF(" + key + ",'')))"
	} else {
		sql += " (" + key + ") WHERE " + key + " <> ''"
	}
	return db.Exec(sql).Error
}
