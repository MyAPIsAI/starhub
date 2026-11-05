package staffcli

import (
	"bytes"
	"path/filepath"
	"starhub/config"
	"starhub/model"
	"strings"
	"testing"
)

func TestDeploymentStaffInitializationAndSoleRootRecovery(t *testing.T) {
	path := filepath.Join(t.TempDir(), "staff-cli.db")
	env := map[string]string{"STARHUB_DB": path, "STARHUB_STAFF_PASSWORD": "Temporary-CLI-password-123"}
	getenv := func(key string) string { return env[key] }
	var output bytes.Buffer
	t.Cleanup(func() { _ = model.CloseDB(); _ = config.ApplyStartup(config.Startup{}) })
	handled, err := Run([]string{"staff", "init", "--username", "root"}, getenv, &output)
	if !handled || err != nil {
		t.Fatalf("CLI initialization: %v", err)
	}
	if strings.Contains(output.String(), env["STARHUB_STAFF_PASSWORD"]) {
		t.Fatal("CLI printed a password")
	}
	if _, err := Run([]string{"staff", "init", "--username", "another-root"}, getenv, &output); err == nil {
		t.Fatal("CLI created a second initial Root")
	}
	if _, err := Run([]string{"staff", "reset-2fa", "--username", "root"}, getenv, &output); err != nil {
		t.Fatal(err)
	}
	if err := model.InitDB(path); err != nil {
		t.Fatal(err)
	}
	root, err := model.GetStaffByUsername("root")
	if err != nil || root.AccountKind != "staff" || !root.MustChangePassword || root.SecurityVersion != 1 {
		t.Fatal("CLI recovery lost staff state")
	}
	_ = model.CloseDB()
}
