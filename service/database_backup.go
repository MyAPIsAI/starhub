package service

import (
	"context"
	"errors"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"time"

	"starhub/model"
)

// DatabaseBackupInterval is opt-in so existing externally managed backups keep
// their ownership. Network databases must use their native backup facilities.
func DatabaseBackupInterval() (time.Duration, error) {
	raw := os.Getenv("STARHUB_BACKUP_INTERVAL")
	if raw == "" {
		return 0, nil
	}
	d, err := time.ParseDuration(raw)
	if err != nil || d < time.Minute {
		return 0, errors.New("STARHUB_BACKUP_INTERVAL must be at least 1m")
	}
	if model.CurrentDialect() != "sqlite" {
		return 0, errors.New("automatic application snapshots require SQLite; use external network database backups")
	}
	directory := os.Getenv("STARHUB_BACKUP_DIR")
	info, err := os.Stat(directory)
	if err != nil || !info.IsDir() {
		return 0, errors.New("automatic backup requires an existing STARHUB_BACKUP_DIR")
	}
	if _, err := backupKeep(); err != nil {
		return 0, err
	}
	return d, nil
}

func backupKeep() (int, error) {
	raw := os.Getenv("STARHUB_BACKUP_KEEP")
	if raw == "" {
		return 7, nil
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n < 1 || n > 10000 {
		return 0, errors.New("STARHUB_BACKUP_KEEP must be between 1 and 10000")
	}
	return n, nil
}

func pruneDatabaseBackups(directory string, keep int) error {
	entries, err := os.ReadDir(directory)
	if err != nil {
		return err
	}
	pattern := regexp.MustCompile(`^starhub-backup-[0-9]{8}-[0-9]{6}-[a-f0-9-]{36}\.db$`)
	var names []string
	for _, entry := range entries {
		if entry.Type().IsRegular() && pattern.MatchString(entry.Name()) {
			names = append(names, entry.Name())
		}
	}
	sort.Sort(sort.Reverse(sort.StringSlice(names)))
	for i := keep; i < len(names); i++ {
		if err := os.Remove(filepath.Join(directory, names[i])); err != nil {
			return err
		}
	}
	return nil
}

func RunDatabaseBackups(ctx context.Context) {
	interval, err := DatabaseBackupInterval()
	if err != nil || interval == 0 {
		return
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		backupCtx, cancel := context.WithTimeout(ctx, 5*time.Minute)
		_, err := model.CreateSQLiteBackup(backupCtx, os.Getenv("STARHUB_BACKUP_DIR"))
		cancel()
		if err == nil {
			keep, _ := backupKeep()
			err = pruneDatabaseBackups(os.Getenv("STARHUB_BACKUP_DIR"), keep)
		}
		if err != nil && ctx.Err() == nil {
			deliverAlert(AlertEvent{Type: "database_backup_failure", Severity: "critical", Message: "Database snapshot or retention failed; inspect backup storage", Timestamp: time.Now().Unix()})
		}
		if err == nil {
			log.Print("verified database snapshot completed")
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}
