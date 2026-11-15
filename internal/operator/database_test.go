package operator

import (
	"os"
	"path/filepath"
	"testing"

	"starhub/model"
)

func TestOperatorDatabaseRejectsMissingFilesAndDriverAmbiguity(t *testing.T) {
	t.Setenv("STARHUB_CONFIG", "")
	t.Setenv("STARHUB_DB_DRIVER", "sqlite")
	t.Setenv("STARHUB_DB_DSN", "")
	path := filepath.Join(t.TempDir(), "missing.db")
	if OpenDatabase(path) == nil {
		t.Fatal("missing database created")
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("operator changed the filesystem", err)
	}
	if err := model.InitDB(path); err != nil {
		t.Fatal(err)
	}
	_ = model.CloseDB()
	if err := OpenDatabase(path); err != nil {
		t.Fatal(err)
	}
	_ = model.CloseDB()
	t.Setenv("STARHUB_DB_DRIVER", "postgres")
	t.Setenv("STARHUB_DB_DSN", "host=unreachable.invalid password=private-secret")
	if err := OpenDatabase(path); err == nil {
		t.Fatal("SQLite path silently ignored for network driver")
	}
}
