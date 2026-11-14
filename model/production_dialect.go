package model

import "gorm.io/gorm"

// Expressions are assembled only from internal column names, never request data.
func productionPayloadLength(db *gorm.DB, column string) string {
	if db.Dialector.Name() == "sqlite" {
		return "length(CAST(" + column + " AS BLOB))"
	}
	return "octet_length(" + column + ")"
}

func productionQuote(db *gorm.DB, name string) string {
	stmt := &gorm.Statement{DB: db}
	return stmt.Quote(name)
}
