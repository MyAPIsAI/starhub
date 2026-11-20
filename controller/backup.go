package controller

import (
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"starhub/common"
	"starhub/model"
)

// BackupDatabase handles POST /api/backup (AdminAuth).
// Triggers an online SQLite backup using VACUUM INTO, which produces
// a consistent atomic snapshot without blocking writes. The backup
// file is saved to the configured backup directory or the current
// working directory. MySQL and PostgreSQL do not support VACUUM INTO;
// callers should use external tools instead:
//
//	mysqldump --single-transaction --routines --triggers starhub > backup.sql
//	pg_dump --format=custom --no-owner starhub > backup.dump
func BackupDatabase(w http.ResponseWriter, r *http.Request) {
	if model.CurrentDialect() != "sqlite" {
		common.ApiError(w, http.StatusNotImplemented, "online backup is only available for SQLite; use external tools: "+
			"mysqldump --single-transaction --routines --triggers <db> > backup.sql (MySQL) or "+
			"pg_dump --format=custom --no-owner <db> > backup.dump (PostgreSQL)")
		return
	}
	backupDir := os.Getenv("STARHUB_BACKUP_DIR")
	if backupDir == "" {
		backupDir = "."
	}
	filename := "starhub-backup-" + time.Now().UTC().Format("20060102-150405") + ".db"
	path := filepath.Join(backupDir, filename)

	// VACUUM INTO creates a new database file with a consistent snapshot.
	// It works in WAL mode without interrupting concurrent readers/writers.
	result := model.DB.Exec("VACUUM INTO ?", path)
	if result.Error != nil {
		common.ApiError(w, http.StatusInternalServerError, "backup failed: "+result.Error.Error())
		return
	}

	info, err := os.Stat(path)
	size := int64(0)
	if err == nil {
		size = info.Size()
	}

	common.ApiSuccess(w, map[string]any{
		"backup":    filename,
		"path":      path,
		"size":      strconv.FormatInt(size, 10),
		"timestamp": time.Now().Unix(),
	})
}
