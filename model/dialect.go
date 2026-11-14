package model

import "fmt"

// currentDialect tracks the active database driver so SQL expressions
// can branch on dialect. Defaults to "sqlite" to preserve existing
// behavior until InitDB configures the real driver.
var currentDialect = "sqlite"

// SetDialect records the active database driver. Call this from InitDB
// before running any queries that rely on dialect-aware expressions.
func SetDialect(driver string) {
	currentDialect = driver
}

// CurrentDialect returns the active database driver name.
func CurrentDialect() string {
	return currentDialect
}

// DateFromUnix returns a SQL expression that formats a Unix epoch integer
// as a UTC 'YYYY-MM-DD', independent of the database session timezone.
//
// SQLite:      date(expr, 'unixepoch')
// PostgreSQL:  TO_CHAR(TO_TIMESTAMP(expr) AT TIME ZONE 'UTC', 'YYYY-MM-DD')
// MySQL:       DATE_FORMAT(TIMESTAMPADD(SECOND, expr, epoch), '%Y-%m-%d')
func DateFromUnix(epochExpr string) string {
	switch currentDialect {
	case "postgres":
		return fmt.Sprintf("TO_CHAR(TO_TIMESTAMP(%s) AT TIME ZONE 'UTC', 'YYYY-MM-DD')", epochExpr)
	case "mysql":
		return fmt.Sprintf("DATE_FORMAT(TIMESTAMPADD(SECOND, %s, '1970-01-01 00:00:00'), '%%Y-%%m-%%d')", epochExpr)
	default:
		return fmt.Sprintf("date(%s, 'unixepoch')", epochExpr)
	}
}
