package model

import (
	"context"
	"errors"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func VerifySQLiteSnapshot(ctx context.Context, path string) error {
	abs, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	u := url.URL{Scheme: "file", Path: filepath.ToSlash(abs), RawQuery: "mode=ro"}
	if filepath.VolumeName(abs) != "" {
		u.Path = "/" + u.Path
	}
	db, err := gorm.Open(sqlite.Open(u.String()), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		return errors.New("snapshot could not be opened read-only")
	}
	pool, err := db.DB()
	if err != nil {
		return err
	}
	defer pool.Close()
	var checks []string
	if err := db.WithContext(ctx).Raw("PRAGMA integrity_check").Scan(&checks).Error; err != nil {
		return err
	}
	if len(checks) != 1 || checks[0] != "ok" {
		return errors.New("snapshot integrity check failed")
	}
	return nil
}

// CreateSQLiteBackup publishes only a completed, verified standalone snapshot.
func CreateSQLiteBackup(ctx context.Context, directory string) (string, error) {
	if CurrentDialect() != "sqlite" {
		return "", errors.New("network databases require external backup tooling")
	}
	info, err := os.Stat(directory)
	if err != nil || !info.IsDir() {
		return "", errors.New("existing backup directory required")
	}
	name := "starhub-backup-" + time.Now().UTC().Format("20060102-150405") + "-" + uuid.NewString() + ".db"
	final := filepath.Join(directory, name)
	stage := final + ".partial"
	defer os.Remove(stage)
	if err := DB.WithContext(ctx).Exec("VACUUM INTO ?", stage).Error; err != nil {
		return "", err
	}
	if err := os.Chmod(stage, 0600); err != nil {
		return "", err
	}
	if err := VerifySQLiteSnapshot(ctx, stage); err != nil {
		return "", err
	}
	file, err := os.OpenFile(stage, os.O_RDWR, 0600)
	if err != nil {
		return "", err
	}
	err = file.Sync()
	closeErr := file.Close()
	if err != nil {
		return "", err
	}
	if closeErr != nil {
		return "", closeErr
	}
	if err := os.Link(stage, final); err != nil {
		return "", err
	}
	return final, nil
}

// RestoreSQLiteSnapshot creates a new database file; switching deployments to it
// requires stopping the server. Existing files are never replaced or removed.
func RestoreSQLiteSnapshot(ctx context.Context, source, destination string) (err error) {
	if err := VerifySQLiteSnapshot(ctx, source); err != nil {
		return err
	}
	for _, suffix := range []string{"-wal", "-journal"} {
		if info, e := os.Stat(source + suffix); e == nil && info.Size() > 0 {
			return errors.New("restore requires a standalone immutable snapshot")
		}
	}
	in, err := os.Open(source)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(destination, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return err
	}
	defer func() {
		_ = out.Close()
		if err != nil {
			_ = os.Remove(destination)
		}
	}()
	if _, err = io.Copy(out, in); err != nil {
		return err
	}
	if err = out.Sync(); err != nil {
		return err
	}
	if err = out.Close(); err != nil {
		return err
	}
	return VerifySQLiteSnapshot(ctx, destination)
}
