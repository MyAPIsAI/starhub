package model

import "fmt"

// CurrentDialect reflects the live connection, never a failed initialization.
func CurrentDialect() string {
	if DB != nil && DB.Dialector != nil {
		return DB.Dialector.Name()
	}
	return "sqlite"
}

// DateFromUnix returns a SQL expression that formats a Unix epoch integer
// as a UTC 'YYYY-MM-DD', independent of the database session timezone.
//
// SQLite:      date(expr, 'unixepoch')
// PostgreSQL:  TO_CHAR(TO_TIMESTAMP(expr) AT TIME ZONE 'UTC', 'YYYY-MM-DD')
// MySQL:       DATE_FORMAT(TIMESTAMPADD(SECOND, expr, epoch), '%Y-%m-%d')
func DateFromUnix(epochExpr string) string {
	switch CurrentDialect() {
	case "postgres":
		return fmt.Sprintf("TO_CHAR(TO_TIMESTAMP(%s) AT TIME ZONE 'UTC', 'YYYY-MM-DD')", epochExpr)
	case "mysql":
		return fmt.Sprintf("DATE_FORMAT(TIMESTAMPADD(SECOND, %s, '1970-01-01 00:00:00'), '%%Y-%%m-%%d')", epochExpr)
	default:
		return fmt.Sprintf("date(%s, 'unixepoch')", epochExpr)
	}
}
