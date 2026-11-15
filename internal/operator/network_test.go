package operator

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"starhub/model"
)

// These commands run only against explicitly supplied disposable databases.
func TestNetworkOperatorConfiguration(t *testing.T) {
	if os.Getenv("STARHUB_TEST_MYSQL_DSN") == "" && os.Getenv("STARHUB_TEST_POSTGRES_DSN") == "" {
		t.Skip("disposable database DSN not supplied")
	}
	binaries := map[string]string{}
	for _, name := range []string{"staff-account", "credential-keys", "reconcile-day"} {
		path := filepath.Join(t.TempDir(), name+".exe")
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		output, err := exec.CommandContext(ctx, "go", "build", "-o", path, "../../cmd/"+name).CombinedOutput()
		cancel()
		if err != nil {
			t.Fatalf("build %s: %v: %s", name, err, output)
		}
		binaries[name] = path
	}
	for driver, env := range map[string]string{"mysql": "STARHUB_TEST_MYSQL_DSN", "postgres": "STARHUB_TEST_POSTGRES_DSN"} {
		t.Run(driver, func(t *testing.T) {
			dsn := os.Getenv(env)
			if dsn == "" {
				t.Skip("disposable database DSN not supplied")
			}
			t.Setenv("STARHUB_DB_DRIVER", driver)
			t.Setenv("STARHUB_DB_DSN", dsn)
			t.Setenv("STARHUB_CONFIG", "")
			t.Setenv("STARHUB_REGION_PROFILE", "")
			t.Setenv("STARHUB_CRYPTO_SECRET", "p27-disposable-operator-test-secret")
			if err := OpenDatabase(""); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() {
				tables, err := model.DB.Migrator().GetTables()
				if err != nil {
					t.Error(err)
				}
				for _, table := range tables {
					if err := model.DB.Migrator().DropTable(table); err != nil {
						t.Error(err)
					}
				}
				_ = model.CloseDB()
			})
			run := func(command string, args ...string) {
				t.Helper()
				ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
				defer cancel()
				cmd := exec.CommandContext(ctx, binaries[command], args...)
				cmd.Env = os.Environ()
				if output, err := cmd.CombinedOutput(); err != nil {
					t.Fatalf("%s: %v: %s", command, err, output)
				}
			}
			t.Setenv("STARHUB_OPERATOR_PASSWORD", "OperatorInitial93")
			run("staff-account", "-action", "initialize", "-username", "operator-root", "-reason", "Disposable integration fixture")
			u, err := model.GetUserByUsername("operator-root")
			if err != nil {
				t.Fatal(err)
			}
			channel := &model.Channel{Name: "operator", Key: "synthetic-upstream-key", Models: "fixture"}
			if err := model.InsertChannel(channel); err != nil {
				t.Fatal(err)
			}
			run("credential-keys", "-rotate", "-reason", "Disposable rotation verification")
			fresh, err := model.GetChannelByID(channel.ID)
			if err != nil || fresh.Key != channel.Key {
				t.Fatal("command changed credential identity", err)
			}
			run("staff-account", "-action", "reset-2fa", "-username", u.Username, "-reason", "Disposable recovery verification")
			run("reconcile-day", "-user", "1", "-date", "2026-11-10", "-timezone", "UTC", "-reason", "Disposable historical report")
		})
	}
}
